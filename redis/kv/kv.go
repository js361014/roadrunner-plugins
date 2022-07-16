package kv

import (
	"context"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/js361014/api/v2/plugins/config"
	kvv1 "github.com/js361014/api/v2/proto/kv/v1beta"
	"github.com/js361014/roadrunner/v2/utils"
	"github.com/spiral/errors"
	"go.uber.org/zap"
)

type driver struct {
	universalClient redis.UniversalClient
	log             *zap.Logger
	cfg             *Config
}

func NewRedisDriver(log *zap.Logger, key string, cfgPlugin config.Configurer) (*driver, error) {
	const op = errors.Op("new_redis_driver")

	d := &driver{
		log: log,
	}

	// will be different for every connected driver
	err := cfgPlugin.UnmarshalKey(key, &d.cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}

	if d.cfg == nil {
		return nil, errors.E(op, errors.Errorf("config not found by provided key: %s", key))
	}

	d.cfg.InitDefaults()

	d.universalClient = redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:              d.cfg.Addrs,
		DB:                 d.cfg.DB,
		Username:           d.cfg.Username,
		Password:           d.cfg.Password,
		SentinelPassword:   d.cfg.SentinelPassword,
		MaxRetries:         d.cfg.MaxRetries,
		MinRetryBackoff:    d.cfg.MaxRetryBackoff,
		MaxRetryBackoff:    d.cfg.MaxRetryBackoff,
		DialTimeout:        d.cfg.DialTimeout,
		ReadTimeout:        d.cfg.ReadTimeout,
		WriteTimeout:       d.cfg.WriteTimeout,
		PoolSize:           d.cfg.PoolSize,
		MinIdleConns:       d.cfg.MinIdleConns,
		MaxConnAge:         d.cfg.MaxConnAge,
		PoolTimeout:        d.cfg.PoolTimeout,
		IdleTimeout:        d.cfg.IdleTimeout,
		IdleCheckFrequency: d.cfg.IdleCheckFreq,
		ReadOnly:           d.cfg.ReadOnly,
		RouteByLatency:     d.cfg.RouteByLatency,
		RouteRandomly:      d.cfg.RouteRandomly,
		MasterName:         d.cfg.MasterName,
	})

	return d, nil
}

// Has checks if value exists.
func (d *driver) Has(keys ...string) (map[string]bool, error) {
	const op = errors.Op("redis_driver_has")
	if keys == nil {
		return nil, errors.E(op, errors.NoKeys)
	}

	m := make(map[string]bool, len(keys))
	for _, key := range keys {
		keyTrimmed := strings.TrimSpace(key)
		if keyTrimmed == "" {
			return nil, errors.E(op, errors.EmptyKey)
		}

		exist, err := d.universalClient.Exists(context.Background(), key).Result()
		if err != nil {
			return nil, err
		}
		if exist == 1 {
			m[key] = true
		}
	}
	return m, nil
}

// Get loads key content into slice.
func (d *driver) Get(key string) ([]byte, error) {
	const op = errors.Op("redis_driver_get")
	// to get cases like "  "
	keyTrimmed := strings.TrimSpace(key)
	if keyTrimmed == "" {
		return nil, errors.E(op, errors.EmptyKey)
	}
	return d.universalClient.Get(context.Background(), key).Bytes()
}

// MGet loads content of multiple values (some values might be skipped).
// https://redis.io/commands/mget
// Returns slice with the interfaces with values
func (d *driver) MGet(keys ...string) (map[string][]byte, error) {
	const op = errors.Op("redis_driver_mget")
	if keys == nil {
		return nil, errors.E(op, errors.NoKeys)
	}

	// should not be empty keys
	for _, key := range keys {
		keyTrimmed := strings.TrimSpace(key)
		if keyTrimmed == "" {
			return nil, errors.E(op, errors.EmptyKey)
		}
	}

	m := make(map[string][]byte, len(keys))

	for _, k := range keys {
		cmd := d.universalClient.Get(context.Background(), k)
		if cmd.Err() != nil {
			if cmd.Err() == redis.Nil {
				continue
			}
			return nil, errors.E(op, cmd.Err())
		}

		m[k] = utils.AsBytes(cmd.Val())
	}

	return m, nil
}

// Set sets value with the TTL in seconds
// https://redis.io/commands/set
// Redis `SET key value [expiration]` command.
//
// Use expiration for `SETEX`-like behavior.
// Zero expiration means the key has no expiration time.
func (d *driver) Set(items ...*kvv1.Item) error {
	const op = errors.Op("redis_driver_set")
	if items == nil {
		return errors.E(op, errors.NoKeys)
	}
	now := time.Now()
	for _, item := range items {
		if item == nil {
			return errors.E(op, errors.EmptyKey)
		}

		if item.Timeout == "" {
			err := d.universalClient.Set(context.Background(), item.Key, item.Value, 0).Err()
			if err != nil {
				return err
			}
		} else {
			t, err := time.Parse(time.RFC3339, item.Timeout)
			if err != nil {
				return err
			}
			err = d.universalClient.Set(context.Background(), item.Key, item.Value, t.Sub(now)).Err()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Delete one or multiple keys.
func (d *driver) Delete(keys ...string) error {
	const op = errors.Op("redis_driver_delete")
	if keys == nil {
		return errors.E(op, errors.NoKeys)
	}

	// should not be empty keys
	for _, key := range keys {
		keyTrimmed := strings.TrimSpace(key)
		if keyTrimmed == "" {
			return errors.E(op, errors.EmptyKey)
		}
	}
	return d.universalClient.Del(context.Background(), keys...).Err()
}

// MExpire https://redis.io/commands/expire
// timeout in RFC3339
func (d *driver) MExpire(items ...*kvv1.Item) error {
	const op = errors.Op("redis_driver_mexpire")
	now := time.Now()
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.Timeout == "" || strings.TrimSpace(item.Key) == "" {
			return errors.E(op, errors.Str("should set timeout and at least one key"))
		}

		t, err := time.Parse(time.RFC3339, item.Timeout)
		if err != nil {
			return err
		}

		// t guessed to be in future
		// for Redis we use t.Sub, it will result in seconds, like 4.2s
		d.universalClient.Expire(context.Background(), item.Key, t.Sub(now))
	}

	return nil
}

// TTL https://redis.io/commands/ttl
// return time in seconds (float64) for a given keys
func (d *driver) TTL(keys ...string) (map[string]string, error) {
	const op = errors.Op("redis_driver_ttl")
	if keys == nil {
		return nil, errors.E(op, errors.NoKeys)
	}

	// should not be empty keys
	for _, key := range keys {
		keyTrimmed := strings.TrimSpace(key)
		if keyTrimmed == "" {
			return nil, errors.E(op, errors.EmptyKey)
		}
	}

	m := make(map[string]string, len(keys))

	for _, key := range keys {
		duration, err := d.universalClient.TTL(context.Background(), key).Result()
		if err != nil {
			return nil, err
		}

		m[key] = duration.String()
	}
	return m, nil
}

func (d *driver) Clear() error {
	fdb := d.universalClient.FlushDB(context.Background())
	if fdb.Err() != nil {
		return fdb.Err()
	}

	return nil
}

func (d *driver) Stop() {
	// close the connection
	_ = d.universalClient.Close()
}
