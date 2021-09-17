package redis

import (
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/spiral/errors"
	"github.com/spiral/roadrunner-plugins/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/internal/common/kv"
	"github.com/spiral/roadrunner-plugins/v2/internal/common/pubsub"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	redis_kv "github.com/spiral/roadrunner-plugins/v2/redis/kv"
	redis_pubsub "github.com/spiral/roadrunner-plugins/v2/redis/pubsub"
)

const PluginName = "redis"

type Plugin struct {
	sync.RWMutex
	// config for RR integration
	cfgPlugin config.Configurer
	// logger
	log logger.Logger
	// redis universal client
	universalClient redis.UniversalClient

	// fanIn implementation used to deliver messages from all channels to the single websocket point
	stopCh chan struct{}
}

func (p *Plugin) Init(cfg config.Configurer, log logger.Logger) error {
	p.log = log
	p.cfgPlugin = cfg
	p.stopCh = make(chan struct{}, 1)

	return nil
}

func (p *Plugin) Serve() chan error {
	return make(chan error)
}

func (p *Plugin) Stop() error {
	const op = errors.Op("redis_plugin_stop")
	p.stopCh <- struct{}{}

	if p.universalClient != nil {
		err := p.universalClient.Close()
		if err != nil {
			return errors.E(op, err)
		}
	}

	return nil
}

func (p *Plugin) Name() string {
	return PluginName
}

// Available interface implementation
func (p *Plugin) Available() {}

// KVConstruct provides KV storage implementation over the redis plugin
func (p *Plugin) KVConstruct(key string) (kv.Storage, error) {
	const op = errors.Op("redis_plugin_provide")
	st, err := redis_kv.NewRedisDriver(p.log, key, p.cfgPlugin)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return st, nil
}

func (p *Plugin) PSConstruct(key string) (pubsub.PubSub, error) {
	return redis_pubsub.NewPubSubDriver(p.log, key, p.cfgPlugin, p.stopCh)
}
