package db

import (
	"errors"
	"github.com/go-redis/redis/v7"
	"github.com/phpgao/proxy_pool/model"
	"math/rand"
	"strings"
	"time"
)

type redisDB struct {
	PrefixKey string
	client    *redis.Client
	KeyExpire int
}

func (r *redisDB) Test() bool {
	pong, err := r.client.Ping().Result()
	if err != nil {
		logger.WithError(err).Error("error test redis")
		return false
	}

	if pong != "PONG" {
		logger.WithField("pong", pong).Error("error connecting to redis")
		return false
	}
	return true
}

func (r *redisDB) Init() error {
	return nil
}

func (r *redisDB) GetListKey(proxy model.HttpProxy) string {
	return strings.Join([]string{
		r.PrefixKey,
		"list",
		proxy.GetKey(),
	}, ":")
}

func (r *redisDB) Add(proxy model.HttpProxy) bool {
	key := strings.Join([]string{
		r.PrefixKey,
		"list",
		proxy.GetKey(),
	}, ":")
	if !r.KeyExists(key) {
		err := r.client.HMSet(key, proxy.GetProxyHash()).Err()
		if err != nil {
			logger.WithError(err).Error("error add proxy")
			return false
		}
	} else {
		err := r.AddScore(proxy, 10)
		if err != nil {
			logger.WithError(err).Error("error add Score")
			return false
		}
	}
	// add ttl
	err := r.ExpireDefault(key)
	if err != nil {
		logger.WithError(err).Error("error setting expire")
		return false
	}

	return true
}

func (r *redisDB) UpdateSchema(proxy model.HttpProxy) (err error) {
	key := strings.Join([]string{
		r.PrefixKey,
		"list",
		proxy.GetKey(),
	}, ":")
	if !r.KeyExists(key) {
		return errors.New("proxy not exists")
	}

	err = r.client.HSet(key, "Schema", proxy.Schema).Err()
	if err != nil {
		return err
	}

	return
}

func (r *redisDB) Expire(key string, expiration time.Duration) error {
	return r.client.Expire(key, expiration).Err()
}
func (r *redisDB) SetScore(key string, score int) error {
	return r.client.HSet(key, "Score", score).Err()
}
func (r *redisDB) GetScore(key string) int {
	v, err := r.client.HGet(key, "Score").Int()
	if err != nil {
		logger.WithField("key", key).WithError(err).Error("error get score")
		return 0
	}
	return v
}

func (r *redisDB) ExpireDefault(key string) error {
	if r.KeyExpire <= 0 {
		return nil
	}
	return r.client.Expire(key, time.Duration(r.KeyExpire)*time.Second).Err()
}

func (r *redisDB) AddScore(p model.HttpProxy, score int) (err error) {
	key := r.GetListKey(p)

	v := r.GetScore(key)
	rs := v + score

	if rs <= 0 {
		err = r.client.Del(key).Err()
		if err != nil {
			return
		}
		return
	}
	if rs >= 100 {
		rs = 100
		err = r.ExpireDefault(key)
		if err != nil {
			return
		}
	}

	return r.SetScore(key, rs)
}

func (r *redisDB) KeyExists(key string) bool {
	val := r.client.Exists(key).Val()
	if val == 1 {
		return true
	}
	return false
}

func (r *redisDB) Exists(proxy model.HttpProxy) bool {
	key := r.GetListKey(proxy)
	return r.KeyExists(key)
}

func (r *redisDB) GetAll() (proxies []model.HttpProxy) {
	keyPattern := strings.Join([]string{
		r.PrefixKey,
		"list",
		"*",
	}, ":")
	keys, _ := r.client.Keys(keyPattern).Result()
	for _, key := range keys {
		proxy := r.client.HGetAll(key).Val()
		//logger.WithField("proxy", proxy).Info("get all proxy")
		newProxy, err := model.Make(proxy)
		if err != nil {
			logger.WithError(err).Error("error create proxy from map")
			continue
		}
		proxies = append(proxies, newProxy)
	}
	return proxies
}

func (r *redisDB) Get(options map[string]string) (proxies []model.HttpProxy, err error) {
	all := r.GetAll()
	filters, err := model.GetNewFilter(options)
	if err != nil {
		return
	}
	if len(filters) > 0 {
		for _, p := range all {
			if Match(filters, p) {
				proxies = append(proxies, p)
			}
		}
	} else {
		proxies = all
	}
	return
}

func Match(filters []func(model.HttpProxy) bool, p model.HttpProxy) bool {
	for _, fc := range filters {
		if fc(p) == false {
			return false
		}
	}

	return true
}

func (r *redisDB) Remove(proxy model.HttpProxy) (rs bool, err error) {
	key := r.GetListKey(proxy)
	if r.KeyExists(key) {
		err = r.client.Del(key).Err()
		if err != nil {
			logger.WithError(err).WithField("key", key).Error("error deleting key")
			return false, err
		}
	}
	return true, nil
}

func (r *redisDB) Random() (p model.HttpProxy, err error) {
	keyPattern := strings.Join([]string{
		r.PrefixKey,
		"list",
		"*",
	}, ":")
	keys, err := r.client.Keys(keyPattern).Result()
	if err != nil {
		logger.WithError(err).WithField("keys", keys).Error("error get keys from redis")
		return
	}
	//rand.Seed(time.Now().Unix())
	key := keys[rand.Intn(len(keys))]
	proxy := r.client.HGetAll(key).Val()
	//logger.WithField("proxy", proxy).Info("get all proxy")
	newProxy, err := model.Make(proxy)
	if err != nil {
		logger.WithError(err).Error("error create proxy from map")
		return
	}
	return newProxy, nil
}

func (r *redisDB) Len() int {
	keyPattern := strings.Join([]string{
		r.PrefixKey,
		"list",
		"*",
	}, ":")
	logger.WithField("keyPattern", keyPattern).Debug("redis cmd keys")
	keys, _ := r.client.Keys(keyPattern).Result()
	return len(keys)
}
