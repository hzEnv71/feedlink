package middleware

import (
	"feed/cache"
	"feed/utils"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitByIP 按 IP 做令牌桶限流。
// rate 表示每秒补充令牌数，burst 表示桶容量。
func RateLimitByIP(prefix string, rate float64, burst int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rate <= 0 || burst <= 0 {
			c.Next()
			return
		}
		ip := c.ClientIP()
		if ip == "" {
			ip = "unknown"
		}
		key := fmt.Sprintf("tb:%s:ip:%s", prefix, ip)
		pass, retryAfter, err := allowByTokenBucket(key, rate, burst)
		if err != nil {
			c.Next()
			return
		}
		if !pass {
			utils.Error(c, 429, "请求过于频繁，请稍后再试")
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Abort()
			return
		}
		c.Next()
	}
}

// RateLimitByUser 按 user_id 做令牌桶限流。
func RateLimitByUser(prefix string, rate float64, burst int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rate <= 0 || burst <= 0 {
			c.Next()
			return
		}
		userID := GetCurrentUserID(c)
		if userID == 0 {
			c.Next()
			return
		}
		key := fmt.Sprintf("tb:%s:user:%d", prefix, userID)
		pass, retryAfter, err := allowByTokenBucket(key, rate, burst)
		if err != nil {
			c.Next()
			return
		}
		if !pass {
			utils.Error(c, 429, "操作过于频繁，请稍后再试")
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Abort()
			return
		}
		c.Next()
	}
}

// allowByTokenBucket 使用 Redis 实现令牌桶：
// - tokens: 当前令牌数
// - ts: 上次补充时间戳（毫秒）
// - 按 rate 补充至 burst 上限，每次请求消费 1 个令牌
func allowByTokenBucket(key string, rate float64, burst int) (bool, int, error) {
	nowMs := time.Now().UnixMilli()
	tokensKey := key + ":tokens"
	tsKey := key + ":ts"

	pipe := cache.RedisClient.Pipeline()
	tokensCmd := pipe.Get(cache.Ctx, tokensKey)
	tsCmd := pipe.Get(cache.Ctx, tsKey)
	_, _ = pipe.Exec(cache.Ctx)

	tokens := float64(burst)
	lastTs := nowMs
	if v, err := tokensCmd.Float64(); err == nil {
		tokens = v
	}
	if v, err := tsCmd.Int64(); err == nil {
		lastTs = v
	}

	if nowMs > lastTs {
		deltaSec := float64(nowMs-lastTs) / 1000.0
		tokens = math.Min(float64(burst), tokens+deltaSec*rate)
	}

	if tokens < 1 {
		need := 1 - tokens
		retryAfterSec := int(math.Ceil(need / rate))
		if retryAfterSec < 1 {
			retryAfterSec = 1
		}

		expireSec := int(math.Ceil(float64(burst)/rate)) * 2
		if expireSec < 2 {
			expireSec = 2
		}
		_ = cache.RedisClient.Set(cache.Ctx, tokensKey, tokens, time.Duration(expireSec)*time.Second).Err()
		_ = cache.RedisClient.Set(cache.Ctx, tsKey, nowMs, time.Duration(expireSec)*time.Second).Err()
		return false, retryAfterSec, nil
	}

	tokens -= 1
	expireSec := int(math.Ceil(float64(burst)/rate)) * 2
	if expireSec < 2 {
		expireSec = 2
	}
	_ = cache.RedisClient.Set(cache.Ctx, tokensKey, tokens, time.Duration(expireSec)*time.Second).Err()
	_ = cache.RedisClient.Set(cache.Ctx, tsKey, nowMs, time.Duration(expireSec)*time.Second).Err()

	return true, 0, nil
}
