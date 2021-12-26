package reload

import (
	"os"
	"strings"
	"time"

	"github.com/spiral/errors"
	"github.com/spiral/roadrunner-plugins/v2/api/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/resetter"
	"go.uber.org/zap"
)

// PluginName contains default plugin name.
const (
	PluginName          string = "reload"
	thresholdChanBuffer uint   = 1000
)

type Plugin struct {
	cfg      *Config
	log      *zap.Logger
	watcher  *Watcher
	services map[string]interface{}
	res      *resetter.Plugin
	stopc    chan struct{}
}

// Init controller service
func (s *Plugin) Init(cfg config.Configurer, log *zap.Logger, res *resetter.Plugin) error {
	const op = errors.Op("reload_plugin_init")
	if !cfg.Has(PluginName) {
		return errors.E(op, errors.Disabled)
	}

	err := cfg.UnmarshalKey(PluginName, &s.cfg)
	if err != nil {
		// disable plugin in case of error
		return errors.E(op, errors.Disabled, err)
	}

	s.cfg.InitDefaults()

	s.log = log
	s.res = res
	s.stopc = make(chan struct{}, 1)
	s.services = make(map[string]interface{})

	configs := make([]WatcherConfig, 0, len(s.cfg.Plugins))

	for serviceName, serviceConfig := range s.cfg.Plugins {
		ignored, errIgn := ConvertIgnored(serviceConfig.Ignore)
		if errIgn != nil {
			return errors.E(op, err)
		}
		configs = append(configs, WatcherConfig{
			ServiceName: serviceName,
			Recursive:   serviceConfig.Recursive,
			Directories: serviceConfig.Dirs,
			FilterHooks: func(filename string, patterns []string) error {
				for i := 0; i < len(patterns); i++ {
					if strings.Contains(filename, patterns[i]) {
						return nil
					}
				}
				return errors.E(op, errors.SkipFile)
			},
			Files:        make(map[string]os.FileInfo),
			Ignored:      ignored,
			FilePatterns: append(serviceConfig.Patterns, s.cfg.Patterns...),
		})
	}

	s.watcher, err = NewWatcher(configs, s.log)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

func (s *Plugin) Serve() chan error {
	const op = errors.Op("reload_plugin_serve")
	errCh := make(chan error, 1)
	if s.cfg.Interval < time.Second {
		errCh <- errors.E(op, errors.Str("reload interval is too fast"))
		return errCh
	}

	// make a map with unique services
	// so, if we would have 100 events from http service
	// in map we would see only 1 key, and it's config
	thCh := make(chan struct {
		serviceConfig ServiceConfig
		service       string
	}, thresholdChanBuffer)

	// use the same interval
	timer := time.NewTimer(s.cfg.Interval)

	go func() {
		for e := range s.watcher.Event {
			thCh <- struct {
				serviceConfig ServiceConfig
				service       string
			}{serviceConfig: s.cfg.Plugins[e.service], service: e.service}
		}
	}()

	// map with config by services
	updated := make(map[string]ServiceConfig, len(s.cfg.Plugins))

	go func() {
		for {
			select {
			case cfg := <-thCh:
				// logic is following:
				// restart
				timer.Stop()
				// replace previous value in map by more recent without adding new one
				updated[cfg.service] = cfg.serviceConfig
				// if we are getting a lot of events, we shouldn't restart particular service on each of it (user doing batch move or very fast typing)
				// instead, we are resetting the timer and wait for s.cfg.Interval time
				// If there is no more events, we restart service only once
				timer.Reset(s.cfg.Interval)
			case <-timer.C:
				if len(updated) > 0 {
					for name := range updated {
						err := s.res.Reset(name)
						if err != nil {
							timer.Stop()
							errCh <- errors.E(op, err)
							return
						}
					}
					// zero map
					updated = make(map[string]ServiceConfig, len(s.cfg.Plugins))
				}
			case <-s.stopc:
				timer.Stop()
				return
			}
		}
	}()

	go func() {
		err := s.watcher.StartPolling(s.cfg.Interval)
		if err != nil {
			errCh <- errors.E(op, err)
			return
		}
	}()

	return errCh
}

func (s *Plugin) Stop() error {
	s.watcher.Stop()
	s.stopc <- struct{}{}
	return nil
}

func (s *Plugin) Name() string {
	return PluginName
}
