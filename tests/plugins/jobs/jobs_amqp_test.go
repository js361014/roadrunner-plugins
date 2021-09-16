package jobs

import (
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	endure "github.com/spiral/endure/pkg/container"
	goridgeRpc "github.com/spiral/goridge/v3/pkg/rpc"
	"github.com/spiral/roadrunner-plugins/v2/amqp"
	"github.com/spiral/roadrunner-plugins/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/informer"
	"github.com/spiral/roadrunner-plugins/v2/jobs"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	jobsv1beta "github.com/spiral/roadrunner-plugins/v2/proto/jobs/v1beta"
	"github.com/spiral/roadrunner-plugins/v2/resetter"
	rpcPlugin "github.com/spiral/roadrunner-plugins/v2/rpc"
	"github.com/spiral/roadrunner-plugins/v2/server"
	"github.com/spiral/roadrunner-plugins/v2/tests/mocks"
	jobState "github.com/spiral/roadrunner/v2/state/job"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAMQPInit(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "amqp/.rr-amqp-init.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	// general
	mockLogger.EXPECT().Debug("worker destructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("worker constructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("Started RPC service", "address", "tcp://127.0.0.1:6001", "plugins", gomock.Any()).Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info("pipeline active", "pipeline", "test-2", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info("pipeline active", "pipeline", "test-1", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)

	mockLogger.EXPECT().Warn("pipeline stopped", "pipeline", "test-1", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Warn("pipeline stopped", "pipeline", "test-2", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)

	mockLogger.EXPECT().Info("delivery channel closed, leaving the rabbit listener").Times(2)

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&amqp.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	if err != nil {
		t.Fatal(err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 3)
	stopCh <- struct{}{}
	wg.Wait()
}

func TestAMQPDeclare(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "amqp/.rr-amqp-declare.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	// general
	mockLogger.EXPECT().Debug("worker destructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("worker constructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("Started RPC service", "address", "tcp://127.0.0.1:6001", "plugins", gomock.Any()).Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info("job pushed to the queue", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Info("job processed without errors", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Info("job processing started", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)

	mockLogger.EXPECT().Info("pipeline active", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info("pipeline paused", "pipeline", "test-3", "driver", "amqp", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Warn("pipeline stopped", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info("delivery channel closed, leaving the rabbit listener").Times(1)

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&amqp.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	if err != nil {
		t.Fatal(err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 3)

	t.Run("DeclareAMQPPipeline", declareAMQPPipe)
	t.Run("ConsumeAMQPPipeline", resumePipes("test-3"))
	t.Run("PushAMQPPipeline", pushToPipe("test-3"))
	time.Sleep(time.Second)
	t.Run("PauseAMQPPipeline", pausePipelines("test-3"))
	time.Sleep(time.Second)
	t.Run("DestroyAMQPPipeline", destroyPipelines("test-3"))

	time.Sleep(time.Second * 5)
	stopCh <- struct{}{}
	wg.Wait()
}

func TestAMQPJobsError(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "amqp/.rr-amqp-jobs-err.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	// general
	mockLogger.EXPECT().Debug("worker destructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("worker constructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("Started RPC service", "address", "tcp://127.0.0.1:6001", "plugins", gomock.Any()).Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info("job pushed to the queue", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Info("job processed without errors", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Info("job processing started", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)

	mockLogger.EXPECT().Info("pipeline active", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info("pipeline paused", "pipeline", "test-3", "driver", "amqp", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Error("jobs protocol error", "error", "error", "delay", gomock.Any(), "requeue", gomock.Any()).Times(3)
	mockLogger.EXPECT().Warn("pipeline stopped", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info("delivery channel closed, leaving the rabbit listener").Times(1)

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&amqp.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	if err != nil {
		t.Fatal(err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 3)

	t.Run("DeclareAMQPPipeline", declareAMQPPipe)
	t.Run("ConsumeAMQPPipeline", resumePipes("test-3"))
	t.Run("PushAMQPPipeline", pushToPipe("test-3"))
	time.Sleep(time.Second * 25)
	t.Run("PauseAMQPPipeline", pausePipelines("test-3"))
	t.Run("DestroyAMQPPipeline", destroyPipelines("test-3"))

	time.Sleep(time.Second * 5)
	stopCh <- struct{}{}
	wg.Wait()
}

func TestAMQPNoGlobalSection(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "amqp/.rr-no-global.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&amqp.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	_, err = cont.Serve()
	require.Error(t, err)
}

func TestAMQPStats(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "amqp/.rr-amqp-declare.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	// general
	mockLogger.EXPECT().Debug("worker destructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("worker constructed", "pid", gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("Started RPC service", "address", "tcp://127.0.0.1:6001", "plugins", gomock.Any()).Times(1)
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info("job pushed to the queue", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Info("pipeline active", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(2)
	mockLogger.EXPECT().Info("pipeline paused", "pipeline", "test-3", "driver", "amqp", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info("job processed without errors", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Info("job processing started", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Warn("pipeline stopped", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info("delivery channel closed, leaving the rabbit listener").AnyTimes()

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&amqp.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	if err != nil {
		t.Fatal(err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 3)

	t.Run("DeclareAMQPPipeline", declareAMQPPipe)
	t.Run("ConsumeAMQPPipeline", resumePipes("test-3"))
	t.Run("PushAMQPPipeline", pushToPipe("test-3"))
	time.Sleep(time.Second * 2)
	t.Run("PauseAMQPPipeline", pausePipelines("test-3"))
	time.Sleep(time.Second * 2)
	t.Run("PushAMQPPipeline", pushToPipe("test-3"))
	t.Run("PushPipelineDelayed", pushToPipeDelayed("test-3", 5))

	out := &jobState.State{}
	t.Run("Stats", stats(out))

	assert.Equal(t, out.Pipeline, "test-3")
	assert.Equal(t, out.Driver, "amqp")
	assert.Equal(t, out.Queue, "default")

	assert.Equal(t, int64(1), out.Active)
	assert.Equal(t, int64(1), out.Delayed)
	assert.Equal(t, int64(0), out.Reserved)
	assert.Equal(t, false, out.Ready)

	time.Sleep(time.Second)
	t.Run("ResumePipeline", resumePipes("test-3"))
	time.Sleep(time.Second * 7)

	out = &jobState.State{}
	t.Run("Stats", stats(out))

	assert.Equal(t, out.Pipeline, "test-3")
	assert.Equal(t, out.Driver, "amqp")
	assert.Equal(t, out.Queue, "default")

	assert.Equal(t, int64(0), out.Active)
	assert.Equal(t, int64(0), out.Delayed)
	assert.Equal(t, int64(0), out.Reserved)
	assert.Equal(t, true, out.Ready)

	time.Sleep(time.Second)
	t.Run("DestroyAMQPPipeline", destroyPipelines("test-3"))

	time.Sleep(time.Second * 5)
	stopCh <- struct{}{}
	wg.Wait()
}

func declareAMQPPipe(t *testing.T) {
	conn, err := net.Dial("tcp", "127.0.0.1:6001")
	assert.NoError(t, err)
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))

	pipe := &jobsv1beta.DeclareRequest{Pipeline: map[string]string{
		"driver":          "amqp",
		"name":            "test-3",
		"routing_key":     "test-3",
		"queue":           "default",
		"exchange_type":   "direct",
		"exchange":        "amqp.default",
		"prefetch":        "100",
		"priority":        "3",
		"exclusive":       "true",
		"multiple_ask":    "true",
		"requeue_on_fail": "true",
	}}

	er := &jobsv1beta.Empty{}
	err = client.Call("jobs.Declare", pipe, er)
	assert.NoError(t, err)
}
