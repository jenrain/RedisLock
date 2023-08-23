package redislock

import (
	"fmt"
	"github.com/go-redis/redis"
	"sync"
	"testing"
	"time"
)

func TestRedisLock_TryLock(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
	})
	RLock = NewRedisLock(redisClient, "test", "work1", 5)
	timeNow := time.Now()
	wg := new(sync.WaitGroup)
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			success, err := RLock.TryLock()
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			if !success {
				fmt.Println("TryLock Fail")
			} else {
				defer func() {
					RLock.Unlock()
					fmt.Println("release the lock")
				}()
				fmt.Println("TryLock Success")
			}
		}()
	}
	wg.Wait()
	deltaTime := time.Since(timeNow)
	fmt.Println(deltaTime)
}
