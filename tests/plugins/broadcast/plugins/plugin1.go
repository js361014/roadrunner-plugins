package plugins

import (
	"context"
	"fmt"

	"github.com/roadrunner-server/api/plugins/v2/pubsub"
	"github.com/spiral/errors"
	"go.uber.org/zap"
)

const Plugin1Name = "plugin1"

type Plugin1 struct {
	log    *zap.Logger
	b      pubsub.Broadcaster
	driver pubsub.SubReader
	ctx    context.Context
	cancel context.CancelFunc
}

func (p *Plugin1) Init(log *zap.Logger, b pubsub.Broadcaster) error {
	p.log = new(zap.Logger)
	*p.log = *log
	p.b = b
	p.ctx, p.cancel = context.WithCancel(context.Background())
	return nil
}

func (p *Plugin1) Serve() chan error {
	errCh := make(chan error, 1)

	var err error
	p.driver, err = p.b.GetDriver("test")
	if err != nil {
		errCh <- err
		return errCh
	}

	err = p.driver.Subscribe("1", "foo", "foo2", "foo3")
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			msg, err := p.driver.Next(p.ctx)
			if err != nil {
				if errors.Is(errors.TimeOut, err) {
					return
				}
				errCh <- err
				return
			}

			if msg == nil {
				continue
			}

			p.log.Info(fmt.Sprintf("%s: %s", Plugin1Name, *msg))
		}
	}()

	return errCh
}

func (p *Plugin1) Stop() error {
	_ = p.driver.Unsubscribe("1", "foo")
	_ = p.driver.Unsubscribe("1", "foo2")
	_ = p.driver.Unsubscribe("1", "foo3")
	p.cancel()
	return nil
}

func (p *Plugin1) Name() string {
	return Plugin1Name
}
