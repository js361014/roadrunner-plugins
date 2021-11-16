package amqp

import (
	"github.com/spiral/roadrunner-plugins/v2/amqp/amqpjobs"
	"github.com/spiral/roadrunner-plugins/v2/api/jobs"
	"github.com/spiral/roadrunner-plugins/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/jobs/pipeline"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	priorityqueue "github.com/spiral/roadrunner/v2/priority_queue"
)

const (
	pluginName string = "amqp"
)

type Plugin struct {
	log logger.Logger
	cfg config.Configurer
}

func (p *Plugin) Init(log logger.Logger, cfg config.Configurer) error {
	p.log = log
	p.cfg = cfg
	return nil
}

func (p *Plugin) Name() string {
	return pluginName
}

func (p *Plugin) Available() {}

func (p *Plugin) ConsumerFromConfig(configKey string, pq priorityqueue.Queue) (jobs.Consumer, error) {
	return amqpjobs.NewAMQPConsumer(configKey, p.log, p.cfg, pq)
}

// ConsumerFromPipeline constructs AMQP driver from pipeline
func (p *Plugin) ConsumerFromPipeline(pipe *pipeline.Pipeline, pq priorityqueue.Queue) (jobs.Consumer, error) {
	return amqpjobs.FromPipeline(pipe, p.log, p.cfg, pq)
}
