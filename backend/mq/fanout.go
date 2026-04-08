package mq

import (
	"encoding/json"
	"feed-system/cache"
	"feed-system/config"
	"feed-system/models"
	"log"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// FeedMessage 推送消息
type FeedMessage struct {
	FeedID    uint    `json:"feed_id"`
	AuthorID  uint    `json:"author_id"`
	Timestamp float64 `json:"timestamp"`
}

var (
	publishConn  *amqp.Connection
	publishChan  *amqp.Channel
	publishQueue amqp.Queue
	publishMu    sync.Mutex
)

// InitMQ 初始化RabbitMQ
func InitMQ() error {
	if err := initPublisher(); err != nil {
		return err
	}

	consumerCount := config.AppConfig.RabbitMQ.Consumers
	if consumerCount <= 0 {
		consumerCount = 10
	}

	for i := 0; i < consumerCount; i++ {
		go consumerLoop(i)
	}

	log.Printf("RabbitMQ initialized with %d consumers", consumerCount)
	return nil
}

func initPublisher() error {
	cfg := config.AppConfig.RabbitMQ

	conn, err := amqp.Dial(cfg.URL())
	if err != nil {
		return err
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}

	queueName := cfg.Queue
	if queueName == "" {
		queueName = "feed_fanout_queue"
	}

	q, err := ch.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	publishConn = conn
	publishChan = ch
	publishQueue = q
	return nil
}

// PublishFeed 发布Feed推送任务
func PublishFeed(feedID, authorID uint) {
	msg := FeedMessage{
		FeedID:    feedID,
		AuthorID:  authorID,
		Timestamp: float64(time.Now().UnixMilli()),
	}

	body, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[MQ] Marshal message failed: %v", err)
		return
	}

	publishMu.Lock()
	defer publishMu.Unlock()

	if publishChan == nil {
		if err := initPublisher(); err != nil {
			log.Printf("[MQ] Re-init publisher failed: %v", err)
			return
		}
	}

	err = publishChan.Publish(
		"",
		publishQueue.Name,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	)
	if err != nil {
		// 尝试重连后重发一次
		_ = resetPublisher()
		if publishChan != nil {
			err = publishChan.Publish(
				"",
				publishQueue.Name,
				false,
				false,
				amqp.Publishing{
					ContentType:  "application/json",
					Body:         body,
					DeliveryMode: amqp.Persistent,
					Timestamp:    time.Now(),
				},
			)
		}
	}

	if err != nil {
		log.Printf("[MQ] Publish feed %d failed: %v", feedID, err)
		return
	}

	log.Printf("[MQ] Feed %d published to queue", feedID)
}

func resetPublisher() error {
	if publishChan != nil {
		_ = publishChan.Close()
		publishChan = nil
	}
	if publishConn != nil {
		_ = publishConn.Close()
		publishConn = nil
	}
	return initPublisher()
}

func consumerLoop(id int) {
	for {
		if err := runConsumer(id); err != nil {
			log.Printf("[MQ Consumer %d] disconnected: %v", id, err)
			time.Sleep(3 * time.Second)
		}
	}
}

func runConsumer(workerID int) error {
	cfg := config.AppConfig.RabbitMQ

	conn, err := amqp.Dial(cfg.URL())
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	queueName := cfg.Queue
	if queueName == "" {
		queueName = "feed_fanout_queue"
	}

	q, err := ch.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	prefetch := cfg.Prefetch
	if prefetch <= 0 {
		prefetch = 50
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return err
	}

	msgs, err := ch.Consume(
		q.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	for d := range msgs {
		var msg FeedMessage
		if err := json.Unmarshal(d.Body, &msg); err != nil {
			log.Printf("[MQ Consumer %d] Invalid message: %v", workerID, err)
			_ = d.Ack(false)
			continue
		}

		processFanout(workerID, msg)
		_ = d.Ack(false)
	}

	return amqp.ErrClosed
}

// processFanout 处理推送逻辑
// 推拉混合策略：
// 1. 普通用户（粉丝数 < 阈值）：推模式 - 将feed推送到所有粉丝的收件箱
// 2. 大V用户（粉丝数 >= 阈值）：拉模式 - 只写入发件箱，粉丝读取时实时拉取合并
func processFanout(workerID int, msg FeedMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[MQ Worker %d] Panic recovered: %v", workerID, r)
		}
	}()

	// 先将feed写入作者的发件箱（所有用户都写）
	err := cache.AddToOutbox(msg.AuthorID, msg.FeedID, msg.Timestamp)
	if err != nil {
		log.Printf("[MQ Worker %d] Add to outbox failed: %v", workerID, err)
	}

	// 判断是否为大V
	isBigV, err := cache.IsBigV(msg.AuthorID)
	if err != nil {
		// Redis查询失败，从数据库查询
		var user models.User
		if dbErr := models.DB.Select("is_big_v").Where("id = ?", msg.AuthorID).First(&user).Error; dbErr == nil {
			isBigV = user.IsBigV
		}
	}

	if isBigV {
		// 大V用户：拉模式，不推送到粉丝收件箱
		log.Printf("[MQ Worker %d] User %d is BigV, skip push, feed %d stored in outbox only",
			workerID, msg.AuthorID, msg.FeedID)
		return
	}

	// 普通用户：推模式，推送到所有粉丝的收件箱
	fanoutToFollowers(workerID, msg)
}

// fanoutToFollowers 推送到粉丝收件箱
func fanoutToFollowers(workerID int, msg FeedMessage) {
	threshold := config.AppConfig.Feed.PushFanLimit

	// 获取粉丝列表
	var followers []models.Follow
	err := models.DB.Where("followed_id = ?", msg.AuthorID).
		Select("user_id").
		Find(&followers).Error
	if err != nil {
		log.Printf("[MQ Worker %d] Query followers failed: %v", workerID, err)
		return
	}

	if len(followers) > threshold {
		// 粉丝数超过阈值，不推送（安全检查，理论上大V已被拦截）
		log.Printf("[MQ Worker %d] User %d has %d followers, exceeds threshold, skip push",
			workerID, msg.AuthorID, len(followers))
		return
	}

	// 批量推送到粉丝收件箱
	successCount := 0
	for _, follower := range followers {
		err := cache.AddToInbox(follower.UserID, msg.FeedID, msg.Timestamp)
		if err != nil {
			log.Printf("[MQ Worker %d] Push to inbox of user %d failed: %v",
				workerID, follower.UserID, err)
			continue
		}

		// 同时写入数据库Timeline表（持久化）
		timeline := models.Timeline{
			UserID:    follower.UserID,
			FeedID:    msg.FeedID,
			AuthorID:  msg.AuthorID,
			CreatedAt: time.Now(),
		}
		if dbErr := models.DB.Create(&timeline).Error; dbErr != nil {
			log.Printf("[MQ Worker %d] Create timeline record failed: %v", workerID, dbErr)
		}

		successCount++
	}

	log.Printf("[MQ Worker %d] Feed %d pushed to %d/%d followers' inbox",
		workerID, msg.FeedID, successCount, len(followers))
}
