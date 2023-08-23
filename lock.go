package redislock

import (
	"context"
	"fmt"
	"github.com/go-redis/redis"
	"time"
)

var RLock *RedisLock

type RedisLock struct {
	key   string
	value string
	// Redis客户端
	redisClient *redis.Client
	// 过期时间
	expiration time.Duration
	// 取消函数
	cancelFunc context.CancelFunc
}

const (
	// PubSubPrefix 发布订阅模式的前缀
	PubSubPrefix = "{redis_lock}_"
	// DefaultGetLockExpiration 处于阻塞的机器默认1秒轮询抢一次锁
	DefaultGetLockExpiration = 1
)

// NewRedisLock 构造函数
func NewRedisLock(redisClient *redis.Client, key, value string, ttl int64) *RedisLock {
	return &RedisLock{
		key:         key,
		value:       value,
		redisClient: redisClient,
		expiration:  time.Duration(ttl) * time.Second,
	}
}

// TryLock 仅尝试一次锁的获取，如果失败，那么不会阻塞，直接返回。
func (lock *RedisLock) TryLock() (bool, error) {
	success, err := lock.redisClient.SetNX(lock.key, lock.value, lock.expiration).Result()
	if err != nil {
		return false, err
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	lock.cancelFunc = cancelFunc
	lock.watchDog(ctx)
	return success, nil
}

// Lock 会造成阻塞直到获取到锁
func (lock *RedisLock) Lock() error {
	for {
		success, err := lock.TryLock()
		if err != nil {
			return err
		}
		// 获取成功
		if success {
			return nil
		} else {
			// 通过获取消息队列中消息的方式获取锁，获取不到会一直阻塞
			err := lock.subscribeLock()
			if err != nil {
				return err
			}
		}
	}
}

// Unlock 解锁
func (lock *RedisLock) Unlock() error {
	// 先判断要解的锁是不是当前key加的
	lua := redis.NewScript(fmt.Sprintf(
		`if redis.call("get", KEYS[1]) == "%s" then 
					return redis.call("del", KEYS[1]) 
				else 
					return 0 
				end`,
		lock.value))
	cmd := lua.Run(lock.redisClient, []string{lock.key})
	res, err := cmd.Result()
	if err != nil {
		return err
	}
	if success, ok := res.(int64); ok {
		// 解锁成功
		if success == 1 {
			// 通知上下文锁已经被释放，重置过期时间
			lock.cancelFunc()
			// 向消息队列中发送消息，锁已经被释放
			err := lock.publishLock()
			if err != nil {
				return err
			}
			return nil
		}
	}
	err = fmt.Errorf("unlock failed: %s", lock.key)
	return err
}

// 返回 PubSubPrefix + key 组成的topic名称
func getPubSubTopic(key string) string {
	return PubSubPrefix + key
}

// 从消息队列中获取锁，获取不到会一直阻塞
func (lock *RedisLock) subscribeLock() error {
	pubSub := lock.redisClient.Subscribe(getPubSubTopic(lock.key))
	_, err := pubSub.Receive()
	if err != nil {
		return err
	}
	deltaTime := DefaultGetLockExpiration * time.Second
	// 阻塞等待直到获取到锁消息或者超时1秒
	select {
	case <-pubSub.Channel():
		return nil
	case <-time.After(deltaTime):
		return fmt.Errorf("timeout")
	}
}

// 向消息队列中发送消息：锁释放成功
func (lock *RedisLock) publishLock() error {
	err := lock.redisClient.Publish(getPubSubTopic(lock.key), "release lock").Err()
	if err != nil {
		return err
	}
	return nil
}

// 看门狗
// 业务执行时间超过 3/2 锁过期时间，续约锁
func (lock *RedisLock) watchDog(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(lock.expiration * 2 / 3)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				lock.redisClient.Expire(lock.key, lock.expiration).Result()
			}
		}
	}()
}
