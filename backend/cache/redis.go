package cache

import (
	"context"
	"feed/config"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	// RedisClient 为全局 Redis 客户端（进程级单例）。
	RedisClient *redis.Client
	// Ctx 为默认上下文；如需超时控制，调用方可自行构造带超时 ctx。
	Ctx = context.Background()
)

const (
	KeyInbox      = "inbox:%d"
	KeyOutbox     = "outbox:%d"
	KeyFeedDetail = "feed:%d"
	KeyUserInfo   = "user:%d"
	KeyFollowers  = "followers:%d"
	KeyFollowing  = "following:%d"
	KeyIsBigV     = "bigv:%d"
)

// InitRedis 初始化 Redis 客户端并执行连通性探测。
func InitRedis() error {
	cfg := config.AppConfig.Redis

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	if _, err := client.Ping(Ctx).Result(); err != nil {
		return fmt.Errorf("connect to redis failed: %w", err)
	}

	RedisClient = client
	log.Println("Redis initialized successfully")
	return nil
}

func AddToInbox(userID, feedID uint, timestamp float64) error {
	key := fmt.Sprintf(KeyInbox, userID)
	if err := addToSortedSet(key, feedID, timestamp); err != nil {
		return err
	}
	trimSortedSetByMaxSize(key, int64(config.AppConfig.Feed.InboxMaxSize))
	return nil
}

func AddToOutbox(userID, feedID uint, timestamp float64) error {
	key := fmt.Sprintf(KeyOutbox, userID)
	if err := addToSortedSet(key, feedID, timestamp); err != nil {
		return err
	}
	trimSortedSetByMaxSize(key, int64(config.AppConfig.Feed.OutboxMaxSize))
	return nil
}

// GetInbox 按时间倒序获取收件箱 feedID 列表。
func GetInbox(userID uint, offset, limit int64) ([]string, error) {
	key := fmt.Sprintf(KeyInbox, userID)
	return getSortedSetByRangeDesc(key, offset, limit)
}

func GetOutbox(userID uint, offset, limit int64) ([]string, error) {
	key := fmt.Sprintf(KeyOutbox, userID)
	return getSortedSetByRangeDesc(key, offset, limit)
}

func GetOutboxByScore(userID uint, minTime, maxTime float64, limit int64) ([]string, error) {
	key := fmt.Sprintf(KeyOutbox, userID)
	return RedisClient.ZRevRangeByScore(Ctx, key, &redis.ZRangeBy{
		Min:   fmt.Sprintf("%f", minTime),
		Max:   fmt.Sprintf("%f", maxTime),
		Count: limit,
	}).Result()
}

func RemoveFromInbox(userID uint, feedID uint) error {
	key := fmt.Sprintf(KeyInbox, userID)
	return RedisClient.ZRem(Ctx, key, feedID).Err()
}

// CacheFeedDetail 缓存 Feed 详情（带随机抖动，防缓存雪崩）。
func CacheFeedDetail(feedID uint, data string) error {
	key := fmt.Sprintf(KeyFeedDetail, feedID)
	ttl := withJitter(24*time.Hour, 10*time.Minute)
	return RedisClient.Set(Ctx, key, data, ttl).Err()
}

func GetFeedDetail(feedID uint) (string, error) {
	key := fmt.Sprintf(KeyFeedDetail, feedID)
	return RedisClient.Get(Ctx, key).Result()
}

// DeleteFeedDetail 主动失效动态详情缓存，用于写操作后的强一致刷新。
func DeleteFeedDetail(feedID uint) error {
	key := fmt.Sprintf(KeyFeedDetail, feedID)
	return RedisClient.Del(Ctx, key).Err()
}

// CacheUserInfo 缓存用户资料（带随机抖动，防缓存雪崩）。
func CacheUserInfo(userID uint, data string) error {
	key := fmt.Sprintf(KeyUserInfo, userID)
	ttl := withJitter(12*time.Hour, 5*time.Minute)
	return RedisClient.Set(Ctx, key, data, ttl).Err()
}

func GetUserInfo(userID uint) (string, error) {
	key := fmt.Sprintf(KeyUserInfo, userID)
	return RedisClient.Get(Ctx, key).Result()
}

func DeleteUserCache(userID uint) error {
	key := fmt.Sprintf(KeyUserInfo, userID)
	return RedisClient.Del(Ctx, key).Err()
}

func SetFollowers(userID uint, followerIDs []any) error {
	key := fmt.Sprintf(KeyFollowers, userID)
	if len(followerIDs) == 0 {
		return nil
	}

	pipe := RedisClient.Pipeline()
	pipe.Del(Ctx, key)
	pipe.SAdd(Ctx, key, followerIDs...)
	pipe.Expire(Ctx, key, withJitter(24*time.Hour, 20*time.Minute))
	_, err := pipe.Exec(Ctx)
	return err
}

// SetFollowing 将关注列表批量写入缓存（带随机抖动，防雪崩）。
func SetFollowing(userID uint, followingIDs []any) error {
	key := fmt.Sprintf(KeyFollowing, userID)
	if len(followingIDs) == 0 {
		return nil
	}

	pipe := RedisClient.Pipeline()
	pipe.Del(Ctx, key)
	pipe.SAdd(Ctx, key, followingIDs...)
	pipe.Expire(Ctx, key, withJitter(24*time.Hour, 20*time.Minute))
	_, err := pipe.Exec(Ctx)
	return err
}

func GetFollowers(userID uint) ([]string, error) {
	key := fmt.Sprintf(KeyFollowers, userID)
	return RedisClient.SMembers(Ctx, key).Result()
}

func AddFollower(userID, followerID uint) error {
	key := fmt.Sprintf(KeyFollowers, userID)
	return RedisClient.SAdd(Ctx, key, followerID).Err()
}

func RemoveFollower(userID, followerID uint) error {
	key := fmt.Sprintf(KeyFollowers, userID)
	return RedisClient.SRem(Ctx, key, followerID).Err()
}

func AddFollowing(userID, followedID uint) error {
	key := fmt.Sprintf(KeyFollowing, userID)
	return RedisClient.SAdd(Ctx, key, followedID).Err()
}

func RemoveFollowing(userID, followedID uint) error {
	key := fmt.Sprintf(KeyFollowing, userID)
	return RedisClient.SRem(Ctx, key, followedID).Err()
}

func GetFollowing(userID uint) ([]string, error) {
	key := fmt.Sprintf(KeyFollowing, userID)
	return RedisClient.SMembers(Ctx, key).Result()
}

func IsFollowing(userID, followedID uint) (bool, error) {
	key := fmt.Sprintf(KeyFollowing, userID)
	return RedisClient.SIsMember(Ctx, key, followedID).Result()
}

func SetBigV(userID uint, isBigV bool) error {
	key := fmt.Sprintf(KeyIsBigV, userID)
	if isBigV {
		return RedisClient.Set(Ctx, key, "1", 0).Err()
	}
	return RedisClient.Del(Ctx, key).Err()
}

// IsBigV 判断用户是否为大V（存在 key 即为 true）。
func IsBigV(userID uint) (bool, error) {
	key := fmt.Sprintf(KeyIsBigV, userID)
	exists, err := RedisClient.Exists(Ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

func addToSortedSet(key string, member uint, score float64) error {
	return RedisClient.ZAdd(Ctx, key, &redis.Z{Score: score, Member: member}).Err()
}

func trimSortedSetByMaxSize(key string, maxSize int64) {
	if maxSize <= 0 {
		return
	}
	_ = RedisClient.ZRemRangeByRank(Ctx, key, 0, -maxSize-1).Err()
}

func getSortedSetByRangeDesc(key string, offset, limit int64) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}
	return RedisClient.ZRevRange(Ctx, key, offset, offset+limit-1).Result()
}

func withJitter(baseTTL, maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return baseTTL
	}
	jitter := time.Duration(rand.Int63n(int64(maxJitter)))
	return baseTTL + jitter
}
