package nats

import (
	"github.com/js361014/api/v2/plugins/config"
	"github.com/js361014/api/v2/plugins/jobs"
	"github.com/js361014/api/v2/plugins/jobs/pipeline"
	"github.com/js361014/roadrunner-plugins/v2/nats/natsjobs"
	priorityqueue "github.com/js361014/roadrunner/v2/priority_queue"
	"go.uber.org/zap"
)

const (
	pluginName string = "nats"
)

type Plugin struct {
	log *zap.Logger
	cfg config.Configurer
}

func (p *Plugin) Init(log *zap.Logger, cfg config.Configurer) error {
	p.log = new(zap.Logger)
	*p.log = *log
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
	return pluginName
}

func (p *Plugin) ConsumerFromConfig(configKey string, queue priorityqueue.Queue) (jobs.Consumer, error) {
	return natsjobs.FromConfig(configKey, p.log, p.cfg, queue)
}

func (p *Plugin) ConsumerFromPipeline(pipe *pipeline.Pipeline, queue priorityqueue.Queue) (jobs.Consumer, error) {
	return natsjobs.FromPipeline(pipe, p.log, p.cfg, queue)
}
