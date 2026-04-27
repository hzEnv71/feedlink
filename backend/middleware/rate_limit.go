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
		pass, retryAfter, err := allowByTokenBucket(key, rate, burst) //获取令牌
		if err != nil {                                               //发送获取令牌失败事件
			c.Next() //继续执行
			return
		}
		if !pass { //如果令牌不足，则返回操作过于频繁事件
			utils.Error(c, 429, "请求过于频繁，请稍后再试")
			c.Header("Retry-After", strconv.Itoa(retryAfter)) //设置重试时间
			c.Abort()
			return //终止请求
		}
		c.Next()
	}
}

// RateLimitByUser 按 user_id 做令牌桶限流。
// rate 表示每秒补充令牌数，burst 表示桶容量。
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
		pass, retryAfter, err := allowByTokenBucket(key, rate, burst) //获取令牌
		if err != nil {                                               //发送获取令牌失败事件
			c.Next() //继续执行
			return
		}
		if !pass { //如果令牌不足，则返回操作过于频繁事件
			utils.Error(c, 429, "操作过于频繁，请稍后再试")
			c.Header("Retry-After", strconv.Itoa(retryAfter)) //设置重试时间
			c.Abort()
			return //终止请求
		}
		c.Next()
	}
}

// RateLimitByFeedFromContext 按 path 中的 feed_id 做令牌桶限流。
// rate 表示每秒补充令牌数，burst 表示桶容量。
func RateLimitByFeedFromContext(prefix string, rate float64, burst int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rate <= 0 || burst <= 0 {
			c.Next()
			return
		}
		feedID, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || feedID == 0 {
			c.Next()
			return
		}
		key := fmt.Sprintf("tb:%s:feed:%d", prefix, feedID)
		pass, retryAfter, err := allowByTokenBucket(key, rate, burst)
		if err != nil {
			c.Next()
			return
		}
		if !pass {
			utils.Error(c, 429, "视频点赞过于频繁，请稍后再试")
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Abort()
			return
		}
		c.Next()
	}
}

// AllowTokenBucket 按任意 key 执行令牌桶限流，返回是否通过与建议等待时间。
func AllowTokenBucket(key string, rate float64, burst int) (bool, int, error) {
	return allowByTokenBucket(key, rate, burst)
}

// allowByTokenBucket 使用 Redis 实现令牌桶：
// - tokens: 当前令牌数
// - ts: 上次补充时间戳（毫秒）
// - 按 rate 补充至 burst 上限，每次请求消费 1 个令牌
func allowByTokenBucket(key string, rate float64, burst int) (bool, int, error) {
	nowMs := time.Now().UnixMilli() //获取当前时间戳
	tokensKey := key + ":tokens"    //获取令牌数key
	tsKey := key + ":ts"            //获取上次补充时间戳key
	pipe := cache.RedisClient.Pipeline()
	tokensCmd := pipe.Get(cache.Ctx, tokensKey) //获取令牌数
	tsCmd := pipe.Get(cache.Ctx, tsKey)         //获取上次补充时间戳
	_, _ = pipe.Exec(cache.Ctx)                 //执行管道
	tokens := float64(burst)                    //初始化令牌数
	lastTs := nowMs                             //初始化上次补充时间戳
	if v, err := tokensCmd.Float64(); err == nil {
		tokens = v //获取令牌数
	}
	if v, err := tsCmd.Int64(); err == nil {
		lastTs = v //获取上次补充时间戳
	}

	if nowMs > lastTs { //如果当前时间戳大于上次补充时间戳，则补充令牌
		deltaSec := float64(nowMs-lastTs) / 1000.0              //计算时间差
		tokens = math.Min(float64(burst), tokens+deltaSec*rate) //计算补充的令牌数
	}

	if tokens < 1 { //如果令牌数小于1，则需要补充令牌
		need := 1 - tokens                           //计算需要补充的令牌数
		retryAfterSec := int(math.Ceil(need / rate)) //计算需要等待的时间
		if retryAfterSec < 1 {                       //如果需要等待的时间小于1秒，则设置为1秒
			retryAfterSec = 1 //设置为1秒
		}

		expireSec := int(math.Ceil(float64(burst)/rate)) * 2 //计算过期时间
		if expireSec < 2 {                                   //如果过期时间小于2秒，则设置为2秒
			expireSec = 2
		}
		_ = cache.RedisClient.Set(cache.Ctx, tokensKey, tokens, time.Duration(expireSec)*time.Second).Err() //设置令牌数
		_ = cache.RedisClient.Set(cache.Ctx, tsKey, nowMs, time.Duration(expireSec)*time.Second).Err()      //设置上次补充时间戳
		return false, retryAfterSec, nil                                                                    //返回false，表示不允许访问，retryAfterSec表示需要等待的时间，nil表示没有错误
	}

	tokens -= 1                                          //消费令牌
	expireSec := int(math.Ceil(float64(burst)/rate)) * 2 //计算过期时间
	if expireSec < 2 {                                   //如果过期时间小于2秒，则设置为2秒
		expireSec = 2
	}
	_ = cache.RedisClient.Set(cache.Ctx, tokensKey, tokens, time.Duration(expireSec)*time.Second).Err() //设置令牌数
	_ = cache.RedisClient.Set(cache.Ctx, tsKey, nowMs, time.Duration(expireSec)*time.Second).Err()      //设置上次补充时间戳

	return true, 0, nil //返回true，表示允许访问，0表示不需要等待，nil表示没有错误
}
