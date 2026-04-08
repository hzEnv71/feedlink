package cache

import (
	"context"
	"feed/config"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	RedisClient *redis.Client
	Ctx         = context.Background()
)

// Redis Key 前缀
const (
	// 用户收件箱 - 存储推送给用户的feed ID列表 (Sorted Set, score=timestamp)
	KeyInbox = "inbox:%d"
	// 用户发件箱 - 存储大V发布的feed ID列表 (Sorted Set, score=timestamp)
	KeyOutbox = "outbox:%d"
	// Feed详情缓存
	KeyFeedDetail = "feed:%d"
	// 用户信息缓存
	KeyUserInfo = "user:%d"
	// 用户粉丝列表
	KeyFollowers = "followers:%d"
	// 用户关注列表
	KeyFollowing = "following:%d"
	// 用户是否为大V
	KeyIsBigV = "bigv:%d"
)

func InitRedis() error {
	cfg := config.AppConfig.Redis

	RedisClient = redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	_, err := RedisClient.Ping(Ctx).Result()
	if err != nil {
		return fmt.Errorf("connect to redis failed: %w", err)
	}

	log.Println("Redis initialized successfully")
	return nil
}

// AddToInbox 将feedID推送到用户收件箱
func AddToInbox(userID uint, feedID uint, timestamp float64) error {
	key := fmt.Sprintf(KeyInbox, userID)
	err := RedisClient.ZAdd(Ctx, key, &redis.Z{
		Score:  timestamp,
		Member: feedID,
	}).Err()
	if err != nil {
		return err
	}

	// 修剪收件箱大小
	maxSize := int64(config.AppConfig.Feed.InboxMaxSize)
	RedisClient.ZRemRangeByRank(Ctx, key, 0, -maxSize-1)

	return nil
}

// AddToOutbox 将feedID添加到用户发件箱
func AddToOutbox(userID uint, feedID uint, timestamp float64) error {
	key := fmt.Sprintf(KeyOutbox, userID)
	err := RedisClient.ZAdd(Ctx, key, &redis.Z{
		Score:  timestamp,
		Member: feedID,
	}).Err()
	if err != nil {
		return err
	}

	// 修剪发件箱大小
	maxSize := int64(config.AppConfig.Feed.OutboxMaxSize)
	RedisClient.ZRemRangeByRank(Ctx, key, 0, -maxSize-1)

	return nil
}

// GetInbox 获取用户收件箱的feed ID列表（按时间倒序）
func GetInbox(userID uint, offset, limit int64) ([]string, error) {
	key := fmt.Sprintf(KeyInbox, userID)
	return RedisClient.ZRevRange(Ctx, key, offset, offset+limit-1).Result()
}

// GetOutbox 获取用户发件箱的feed ID列表（按时间倒序）
func GetOutbox(userID uint, offset, limit int64) ([]string, error) {
	key := fmt.Sprintf(KeyOutbox, userID)
	return RedisClient.ZRevRange(Ctx, key, offset, offset+limit-1).Result()
}

// GetOutboxByScore 获取发件箱中指定时间范围内的feed ID列表
func GetOutboxByScore(userID uint, minTime, maxTime float64, limit int64) ([]string, error) {
	key := fmt.Sprintf(KeyOutbox, userID)
	return RedisClient.ZRevRangeByScore(Ctx, key, &redis.ZRangeBy{
		Min:   fmt.Sprintf("%f", minTime),
		Max:   fmt.Sprintf("%f", maxTime),
		Count: limit,
	}).Result()
}

// RemoveFromInbox 从收件箱移除指定feed
func RemoveFromInbox(userID uint, feedID uint) error {
	key := fmt.Sprintf(KeyInbox, userID)
	return RedisClient.ZRem(Ctx, key, feedID).Err()
}

// CacheFeedDetail 缓存Feed详情
func CacheFeedDetail(feedID uint, data string) error {
	key := fmt.Sprintf(KeyFeedDetail, feedID)
	return RedisClient.Set(Ctx, key, data, 24*time.Hour).Err()
}

// GetFeedDetail 获取Feed详情缓存
func GetFeedDetail(feedID uint) (string, error) {
	key := fmt.Sprintf(KeyFeedDetail, feedID)
	return RedisClient.Get(Ctx, key).Result()
}

// CacheUserInfo 缓存用户信息
func CacheUserInfo(userID uint, data string) error {
	key := fmt.Sprintf(KeyUserInfo, userID)
	return RedisClient.Set(Ctx, key, data, 12*time.Hour).Err()
}

// GetUserInfo 获取用户信息缓存
func GetUserInfo(userID uint) (string, error) {
	key := fmt.Sprintf(KeyUserInfo, userID)
	return RedisClient.Get(Ctx, key).Result()
}

// DeleteUserCache 删除用户缓存
func DeleteUserCache(userID uint) error {
	key := fmt.Sprintf(KeyUserInfo, userID)
	return RedisClient.Del(Ctx, key).Err()
}

// SetFollowers 设置粉丝列表到Redis
func SetFollowers(userID uint, followerIDs []interface{}) error {
	key := fmt.Sprintf(KeyFollowers, userID)
	if len(followerIDs) == 0 {
		return nil
	}
	pipe := RedisClient.Pipeline()
	pipe.Del(Ctx, key)
	pipe.SAdd(Ctx, key, followerIDs...)
	pipe.Expire(Ctx, key, 24*time.Hour)
	_, err := pipe.Exec(Ctx)
	return err
}

// GetFollowers 获取粉丝列表
func GetFollowers(userID uint) ([]string, error) {
	key := fmt.Sprintf(KeyFollowers, userID)
	return RedisClient.SMembers(Ctx, key).Result()
}

// AddFollower 添加粉丝
func AddFollower(userID, followerID uint) error {
	key := fmt.Sprintf(KeyFollowers, userID)
	return RedisClient.SAdd(Ctx, key, followerID).Err()
}

// RemoveFollower 移除粉丝
func RemoveFollower(userID, followerID uint) error {
	key := fmt.Sprintf(KeyFollowers, userID)
	return RedisClient.SRem(Ctx, key, followerID).Err()
}

// AddFollowing 添加关注
func AddFollowing(userID, followedID uint) error {
	key := fmt.Sprintf(KeyFollowing, userID)
	return RedisClient.SAdd(Ctx, key, followedID).Err()
}

// RemoveFollowing 移除关注
func RemoveFollowing(userID, followedID uint) error {
	key := fmt.Sprintf(KeyFollowing, userID)
	return RedisClient.SRem(Ctx, key, followedID).Err()
}

// GetFollowing 获取关注列表
func GetFollowing(userID uint) ([]string, error) {
	key := fmt.Sprintf(KeyFollowing, userID)
	return RedisClient.SMembers(Ctx, key).Result()
}

// IsFollowing 判断是否已关注
func IsFollowing(userID, followedID uint) (bool, error) {
	key := fmt.Sprintf(KeyFollowing, userID)
	return RedisClient.SIsMember(Ctx, key, followedID).Result()
}

// SetBigV 标记用户为大V
func SetBigV(userID uint, isBigV bool) error {
	key := fmt.Sprintf(KeyIsBigV, userID)
	if isBigV {
		return RedisClient.Set(Ctx, key, "1", 0).Err()
	}
	return RedisClient.Del(Ctx, key).Err()
}

// IsBigV 判断是否为大V
func IsBigV(userID uint) (bool, error) {
	key := fmt.Sprintf(KeyIsBigV, userID)
	result, err := RedisClient.Exists(Ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}
