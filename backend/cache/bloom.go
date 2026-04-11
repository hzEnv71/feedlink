package cache

import (
	"feed/models"
	"fmt"
	"hash/fnv"
	"sync/atomic"

	"github.com/go-redis/redis/v8"
)

const (
	bloomUserKey = "bf:user:id"
	bloomFeedKey = "bf:feed:id"
	bloomM       = uint(1 << 24) // 16,777,216 bits (~2MB)
	bloomK       = 6
)

var bloomReady atomic.Bool

func InitBloomFilters() error {
	if RedisClient == nil {
		return fmt.Errorf("redis not initialized")
	}

	var users []models.User
	if err := models.DB.Select("id").Find(&users).Error; err != nil {
		return err
	}
	for _, u := range users {
		_ = bloomAdd(bloomUserKey, u.ID)
	}

	var feeds []models.Feed
	if err := models.DB.Select("id").Find(&feeds).Error; err != nil {
		return err
	}
	for _, f := range feeds {
		_ = bloomAdd(bloomFeedKey, f.ID)
	}

	bloomReady.Store(true)
	return nil
}

func AddUserID(id uint) {
	if id == 0 || RedisClient == nil {
		return
	}
	_ = bloomAdd(bloomUserKey, id)
}

func AddFeedID(id uint) {
	if id == 0 || RedisClient == nil {
		return
	}
	_ = bloomAdd(bloomFeedKey, id)
}

func MightUserExist(id uint) bool {
	if id == 0 {
		return false
	}
	if !bloomReady.Load() {
		return true
	}
	ok, err := bloomMightContain(bloomUserKey, id)
	if err != nil {
		return true
	}
	return ok
}

func MightFeedExist(id uint) bool {
	if id == 0 {
		return false
	}
	if !bloomReady.Load() {
		return true
	}
	ok, err := bloomMightContain(bloomFeedKey, id)
	if err != nil {
		return true
	}
	return ok
}

func bloomAdd(key string, id uint) error {
	offsets := bloomOffsets(id)
	pipe := RedisClient.Pipeline()
	for _, off := range offsets {
		pipe.SetBit(Ctx, key, int64(off), 1)
	}
	_, err := pipe.Exec(Ctx)
	return err
}

func bloomMightContain(key string, id uint) (bool, error) {
	offsets := bloomOffsets(id)
	pipe := RedisClient.Pipeline()
	cmds := make([]*redis.IntCmd, 0, len(offsets))
	for _, off := range offsets {
		cmds = append(cmds, pipe.GetBit(Ctx, key, int64(off)))
	}
	if _, err := pipe.Exec(Ctx); err != nil {
		return false, err
	}
	for _, cmd := range cmds {
		v, err := cmd.Result()
		if err != nil {
			return false, err
		}
		if v == 0 {
			return false, nil
		}
	}
	return true, nil
}

func bloomOffsets(id uint) []uint {
	offsets := make([]uint, 0, bloomK)
	v := fmt.Sprintf("%d", id)
	for i := range bloomK {
		h := fnv.New64a()
		_, _ = h.Write([]byte{byte(i + 1)})
		_, _ = h.Write([]byte(v))
		offsets = append(offsets, uint(h.Sum64()%uint64(bloomM)))
	}
	return offsets
}
