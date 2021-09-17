package memory

import (
	"github.com/spiral/errors"
	"github.com/spiral/roadrunner-plugins/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/internal/common/jobs"
	"github.com/spiral/roadrunner-plugins/v2/internal/common/kv"
	"github.com/spiral/roadrunner-plugins/v2/internal/common/pubsub"
	"github.com/spiral/roadrunner-plugins/v2/jobs/pipeline"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	"github.com/spiral/roadrunner-plugins/v2/memory/memoryjobs"
	"github.com/spiral/roadrunner-plugins/v2/memory/memorykv"
	"github.com/spiral/roadrunner-plugins/v2/memory/memorypubsub"
	"github.com/spiral/roadrunner/v2/events"
	priorityqueue "github.com/spiral/roadrunner/v2/priority_queue"
)

const PluginName string = "memory"

type Plugin struct {
	log logger.Logger
	cfg config.Configurer
}

func (p *Plugin) Init(log logger.Logger, cfg config.Configurer) error {
	p.log = log
	p.cfg = cfg
	return nil
}

func (p *Plugin) Serve() chan error {
	return make(chan error, 1)
}

func (p *Plugin) Stop() error {
	return nil
}

func (p *Plugin) Name() string {
	return PluginName
}

func (p *Plugin) Available() {}

// Drivers implementation

func (p *Plugin) PSConstruct(key string) (pubsub.PubSub, error) {
	return memorypubsub.NewPubSubDriver(p.log, key)
}

func (p *Plugin) KVConstruct(key string) (kv.Storage, error) {
	const op = errors.Op("memory_plugin_construct")
	st, err := memorykv.NewInMemoryDriver(key, p.log, p.cfg)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return st, nil
}

// JobsConstruct creates new ephemeral consumer from the configuration
func (p *Plugin) JobsConstruct(configKey string, e events.Handler, pq priorityqueue.Queue) (jobs.Consumer, error) {
	return memoryjobs.NewJobBroker(configKey, p.log, p.cfg, e, pq)
}

// FromPipeline creates new ephemeral consumer from the provided pipeline
func (p *Plugin) FromPipeline(pipeline *pipeline.Pipeline, e events.Handler, pq priorityqueue.Queue) (jobs.Consumer, error) {
	return memoryjobs.FromPipeline(pipeline, p.log, e, pq)
}
