package redislock

import (
	"context"
	"github.com/garyburd/redigo/redis"
)

var redisLock *RedisLock

type RedisLock struct {
	key   string
	value string
	// Redis 客户端
	redisClient *redis.ConnWithTimeout
	// 过期时间
	ttl int64
	// 取消函数
	cancel context.CancelFunc
}

const (
	// PubSubPrefix 发布订阅模式的前缀
	PubSubPrefix = "{redis_lock}_"
	// DefaultConnExpiration 默认获取不到连接的过期时间为30秒
	DefaultConnExpiration = 30
	// DefaultLockExpiration 默认获取不到分布式锁的超时时间为30秒
	DefaultLockExpiration = 30
)

func NewRedisLock(redisClient *redis.ConnWithTimeout, key, value string, ttl int64) *RedisLock {
	return &RedisLock{
		key:         key,
		value:       value,
		redisClient: redisClient,
		ttl:         ttl,
	}
}
