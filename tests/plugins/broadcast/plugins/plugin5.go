package plugins

import (
	"context"
	"fmt"

	"github.com/js361014/api/v2/plugins/pubsub"
	"github.com/spiral/errors"
	"go.uber.org/zap"
)

const Plugin5Name = "plugin5"

type Plugin5 struct {
	log    *zap.Logger
	b      pubsub.Broadcaster
	driver pubsub.SubReader
	ctx    context.Context
	cancel context.CancelFunc
}

func (p *Plugin5) Init(log *zap.Logger, b pubsub.Broadcaster) error {
	p.log = new(zap.Logger)
	*p.log = *log
	p.b = b
	p.ctx, p.cancel = context.WithCancel(context.Background())
	return nil
}

func (p *Plugin5) Serve() chan error {
	errCh := make(chan error, 1)

	var err error
	p.driver, err = p.b.GetDriver("test4")
	if err != nil {
		panic(err)
	}

	err = p.driver.Subscribe("5", "foo")
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

			p.log.Info(fmt.Sprintf("%s: %s", Plugin5Name, *msg))
		}
	}()

	return errCh
}

func (p *Plugin5) Stop() error {
	_ = p.driver.Unsubscribe("5", "foo")
	p.cancel()
	return nil
}

func (p *Plugin5) Name() string {
	return Plugin5Name
}
