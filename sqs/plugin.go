package sqs

import (
	"github.com/js361014/api/v2/plugins/config"
	"github.com/js361014/api/v2/plugins/jobs"
	"github.com/js361014/api/v2/plugins/jobs/pipeline"
	"github.com/js361014/roadrunner-plugins/v2/sqs/sqsjobs"
	priorityqueue "github.com/js361014/roadrunner/v2/priority_queue"
	"go.uber.org/zap"
)

const (
	pluginName string = "sqs"
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

func (p *Plugin) Name() string {
	return pluginName
}

func (p *Plugin) ConsumerFromConfig(configKey string, pq priorityqueue.Queue) (jobs.Consumer, error) {
	return sqsjobs.NewSQSConsumer(configKey, p.log, p.cfg, pq)
}

func (p *Plugin) ConsumerFromPipeline(pipe *pipeline.Pipeline, pq priorityqueue.Queue) (jobs.Consumer, error) {
	return sqsjobs.FromPipeline(pipe, p.log, p.cfg, pq)
}
