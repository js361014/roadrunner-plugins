package memcached

import (
	"github.com/spiral/errors"
	"github.com/spiral/roadrunner-plugins/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/internal/common/kv"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	"github.com/spiral/roadrunner-plugins/v2/memcached/memcachedkv"
)

const (
	PluginName     string = "memcached"
	RootPluginName string = "kv"
)

type Plugin struct {
	// config plugin
	cfgPlugin config.Configurer
	// logger
	log logger.Logger
}

func (s *Plugin) Init(log logger.Logger, cfg config.Configurer) error {
	if !cfg.Has(RootPluginName) {
		return errors.E(errors.Disabled)
	}

	s.cfgPlugin = cfg
	s.log = log
	return nil
}

// Name returns plugin user-friendly name
func (s *Plugin) Name() string {
	return PluginName
}

// Available interface implementation
func (s *Plugin) Available() {}

func (s *Plugin) KvFromConfig(key string) (kv.Storage, error) {
	const op = errors.Op("boltdb_plugin_provide")
	st, err := memcachedkv.NewMemcachedDriver(s.log, key, s.cfgPlugin)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return st, nil
}
