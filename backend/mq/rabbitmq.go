package mq

import (
	"encoding/json"
	"feed/cache"
	"feed/config"
	"feed/models"
	"fmt"
	"log"
	"maps"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	headerRetryCount  = "x-retry-count"
	headerMessageKey  = "x-message-key"
	headersContentType = "application/json"
)

// FeedMessage 推送消息
type FeedMessage struct {
	FeedID    uint    `json:"feed_id"`
	AuthorID  uint    `json:"author_id"`
	Timestamp float64 `json:"timestamp"`
}

var (
	publisherConn  *amqp.Connection
	publisherChan  *amqp.Channel
	publisherQueue amqp.Queue
	publisherMu    sync.Mutex
)

// InitMQ 初始化 RabbitMQ，并启动消费者。
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

// initPublisher 初始化发布端连接与通道，并声明完整队列拓扑。
// 失败时由调用方决定重试策略。
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

	if err := declareTopology(ch); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	q, err := declareMainQueue(ch)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	publisherConn = conn
	publisherChan = ch
	publisherQueue = q
	return nil
}

// declareTopology 声明 MQ 拓扑：主队列、重试队列、死信队列。
// 说明：
// - 主队列消费失败后可进入重试或死信流程；
// - 重试队列通过 TTL 到期后回流主队列；
// - 死信队列承接超过重试上限的消息，便于排查与回放。
func declareTopology(ch *amqp.Channel) error {
	if _, err := declareMainQueue(ch); err != nil {
		return err
	}
	if _, err := declareRetryQueue(ch); err != nil {
		return err
	}
	if _, err := declareDeadLetterQueue(ch); err != nil {
		return err
	}
	return nil
}

// declareMainQueue 声明主消费队列。
// 配置 dead-letter-routing-key 指向 DLQ，便于异常兜底。
func declareMainQueue(ch *amqp.Channel) (amqp.Queue, error) {
	cfg := config.AppConfig.RabbitMQ
	args := amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": cfg.DeadLetterQueueName(),
	}
	return ch.QueueDeclare(cfg.QueueName(), true, false, false, false, args)
}

// declareRetryQueue 声明重试队列。
// 消息在该队列等待 retry_delay_ms 后自动回流主队列，实现延迟重试。
func declareRetryQueue(ch *amqp.Channel) (amqp.Queue, error) {
	cfg := config.AppConfig.RabbitMQ
	delay := cfg.RetryDelayMS
	if delay <= 0 {
		delay = 5000
	}
	args := amqp.Table{
		"x-message-ttl":             int32(delay),
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": cfg.QueueName(),
	}
	return ch.QueueDeclare(cfg.RetryQueueName(), true, false, false, false, args)
}

func declareDeadLetterQueue(ch *amqp.Channel) (amqp.Queue, error) {
	cfg := config.AppConfig.RabbitMQ
	return ch.QueueDeclare(cfg.DeadLetterQueueName(), true, false, false, false, nil)
}

// PublishFeed 发布 Feed 推送任务。
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

	publisherMu.Lock()
	defer publisherMu.Unlock()

	if publisherChan == nil {
		if err := initPublisher(); err != nil {
			log.Printf("[MQ] Re-init publisher failed: %v", err)
			return
		}
	}

	msgKey := messageKey(feedID, authorID)
	err = publishToQueue(publisherQueue.Name, body, amqp.Table{
		headerRetryCount: int32(0),
		headerMessageKey: msgKey,
	})
	if err != nil {
		_ = resetPublisher()
		if publisherChan != nil {
			err = publishToQueue(publisherQueue.Name, body, amqp.Table{
				headerRetryCount: int32(0),
				headerMessageKey: msgKey,
			})
		}
	}

	if err != nil {
		log.Printf("[MQ] Publish feed %d failed: %v", feedID, err)
		return
	}

	log.Printf("[MQ] Feed %d published to queue", feedID)
}

// publishToQueue 统一发布函数。
// 所有重试次数、幂等键等元信息通过 headers 透传。
func publishToQueue(queueName string, body []byte, headers amqp.Table) error {
	return publisherChan.Publish(
		"",
		queueName,
		false,
		false,
		amqp.Publishing{
			Headers:      headers,
			ContentType:  headersContentType,
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
		},
	)
}

func resetPublisher() error {
	if publisherChan != nil {
		_ = publisherChan.Close()
		publisherChan = nil
	}
	if publisherConn != nil {
		_ = publisherConn.Close()
		publisherConn = nil
	}
	return initPublisher()
}

func consumerLoop(workerID int) {
	for {
		if err := runConsumer(workerID); err != nil {
			log.Printf("[MQ Consumer %d] disconnected: %v", workerID, err)
			time.Sleep(3 * time.Second)
		}
	}
}

// runConsumer 启动单个消费者：
// - 声明队列拓扑
// - 设置 Qos
// - 订阅主队列并逐条处理
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

	if err := declareTopology(ch); err != nil {
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
		cfg.QueueName(),
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

		processed, err := processDispatch(workerID, msg, d.Headers)
		if err != nil {
			handleRetryOrDeadLetter(workerID, d, err)
			continue
		}
		if processed {
			_ = d.Ack(false)
		}
	}

	return amqp.ErrClosed
}

// processDispatch 执行推拉混合分发核心逻辑，并做幂等保护。
func processDispatch(workerID int, msg FeedMessage, headers amqp.Table) (bool, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[MQ Worker %d] Panic recovered: %v", workerID, r)
		}
	}()

	messageKey := extractMessageKey(msg, headers)
	if messageKey == "" {
		messageKey = messageKeyFromMessage(msg)
	}

	ok, err := tryMarkMessageProcessing(messageKey)
	if err != nil {
		return false, fmt.Errorf("idempotent check failed: %w", err)
	}
	if !ok {
		log.Printf("[MQ Worker %d] Duplicate message ignored: %s", workerID, messageKey)
		return true, nil
	}

	if err := cache.AddToOutbox(msg.AuthorID, msg.FeedID, msg.Timestamp); err != nil {
		return false, fmt.Errorf("add to outbox failed: %w", err)
	}

	isBigV, err := cache.IsBigV(msg.AuthorID)
	if err != nil {
		var user models.User
		if dbErr := models.DB.Select("is_big_v").Where("id = ?", msg.AuthorID).First(&user).Error; dbErr == nil {
			isBigV = user.IsBigV
		}
	}

	if isBigV {
		log.Printf("[MQ Worker %d] User %d is BigV, skip push, feed %d stored in outbox only", workerID, msg.AuthorID, msg.FeedID)
		return true, nil
	}

	if err := dispatchToFollowers(workerID, msg); err != nil {
		return false, err
	}
	return true, nil
}

func dispatchToFollowers(workerID int, msg FeedMessage) error {
	threshold := config.AppConfig.Feed.PushFanLimit

	var followers []models.Follow
	if err := models.DB.Where("followed_id = ?", msg.AuthorID).Select("user_id").Find(&followers).Error; err != nil {
		return fmt.Errorf("query followers failed: %w", err)
	}

	if len(followers) > threshold {
		log.Printf("[MQ Worker %d] User %d has %d followers, exceeds threshold, skip push", workerID, msg.AuthorID, len(followers))
		return nil
	}

	successCount := 0
	for _, follower := range followers {
		if err := cache.AddToInbox(follower.UserID, msg.FeedID, msg.Timestamp); err != nil {
			log.Printf("[MQ Worker %d] Push to inbox of user %d failed: %v", workerID, follower.UserID, err)
			continue
		}

		timeline := models.Timeline{
			UserID:    follower.UserID,
			FeedID:    msg.FeedID,
			AuthorID:  msg.AuthorID,
			CreatedAt: time.Now(),
		}
		if err := models.DB.Create(&timeline).Error; err != nil {
			log.Printf("[MQ Worker %d] Create timeline record failed: %v", workerID, err)
		}

		successCount++
	}

	log.Printf("[MQ Worker %d] Feed %d pushed to %d/%d followers' inbox", workerID, msg.FeedID, successCount, len(followers))
	return nil
}

// handleRetryOrDeadLetter 处理消费失败的消息：
// - 未超过最大重试：投递到重试队列（延迟后回流主队列）
// - 超过最大重试：投递到死信队列（DLQ）
func handleRetryOrDeadLetter(workerID int, d amqp.Delivery, processErr error) {
	cfg := config.AppConfig.RabbitMQ
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	retryCount := extractRetryCount(d.Headers)
	retryCount++

	if publisherChan == nil {
		if err := initPublisher(); err != nil {
			log.Printf("[MQ Worker %d] re-init publisher failed when retry: %v", workerID, err)
			_ = d.Nack(false, true)
			return
		}
	}

	headers := copyHeaders(d.Headers)
	headers[headerRetryCount] = int32(retryCount)

	var routeQueue string
	if retryCount > maxRetries {
		routeQueue = cfg.DeadLetterQueueName()
		log.Printf("[MQ Worker %d] move message to DLQ after %d retries, err=%v", workerID, retryCount-1, processErr)
	} else {
		routeQueue = cfg.RetryQueueName()
		log.Printf("[MQ Worker %d] retry message #%d, err=%v", workerID, retryCount, processErr)
	}

	if err := publishToQueue(routeQueue, d.Body, headers); err != nil {
		log.Printf("[MQ Worker %d] publish retry/dlq failed: %v", workerID, err)
		_ = d.Nack(false, true)
		return
	}

	_ = d.Ack(false)
}

// tryMarkMessageProcessing 尝试设置幂等标记。
// 返回 true 表示“首次处理”，false 表示“重复消息”。
func tryMarkMessageProcessing(messageKey string) (bool, error) {
	ttlMin := config.AppConfig.RabbitMQ.IdempotentTTLMin
	if ttlMin <= 0 {
		ttlMin = 60
	}
	ttl := time.Duration(ttlMin) * time.Minute
	idKey := "mq:idempotent:" + messageKey
	ok, err := cache.RedisClient.SetNX(cache.Ctx, idKey, "1", ttl).Result()
	return ok, err
}

func extractRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	raw, ok := headers[headerRetryCount]
	if !ok {
		return 0
	}
	return parseHeaderInt(raw)
}

func extractMessageKey(msg FeedMessage, headers amqp.Table) string {
	if headers == nil {
		return messageKeyFromMessage(msg)
	}
	if raw, ok := headers[headerMessageKey]; ok {
		if s, ok := raw.(string); ok && s != "" {
			return s
		}
	}
	return messageKeyFromMessage(msg)
}

func messageKey(feedID, authorID uint) string {
	return fmt.Sprintf("feed:%d:author:%d", feedID, authorID)
}

func messageKeyFromMessage(msg FeedMessage) string {
	return messageKey(msg.FeedID, msg.AuthorID)
}

func copyHeaders(src amqp.Table) amqp.Table {
	dst := amqp.Table{}
	maps.Copy(dst, src)
	return dst
}

func parseHeaderInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int8:
		return int(val)
	case int16:
		return int(val)
	case int32:
		return int(val)
	case int64:
		return int(val)
	case uint:
		return int(val)
	case uint8:
		return int(val)
	case uint16:
		return int(val)
	case uint32:
		return int(val)
	case uint64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	default:
		return 0
	}
}
