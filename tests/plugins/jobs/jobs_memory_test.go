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
	jobState "github.com/spiral/roadrunner-plugins/v2/api/jobs"
	jobsv1beta "github.com/spiral/roadrunner-plugins/v2/api/proto/jobs/v1beta"
	"github.com/spiral/roadrunner-plugins/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/informer"
	"github.com/spiral/roadrunner-plugins/v2/jobs"
	"github.com/spiral/roadrunner-plugins/v2/logger"
	"github.com/spiral/roadrunner-plugins/v2/memory"
	"github.com/spiral/roadrunner-plugins/v2/resetter"
	rpcPlugin "github.com/spiral/roadrunner-plugins/v2/rpc"
	"github.com/spiral/roadrunner-plugins/v2/server"
	"github.com/spiral/roadrunner-plugins/v2/tests/mocks"
	"github.com/stretchr/testify/assert"
)

func TestMemoryInit(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Plugin{
		Path:   "memory/.rr-memory-init.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	// general
	mockLogger.EXPECT().Debug("RPC plugin started", "address", "tcp://127.0.0.1:6001", "plugins", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Debug("pipeline active", "driver", "memory", "pipeline", "test-1", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline active", "driver", "memory", "pipeline", "test-2", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline stopped", "driver", "memory", "pipeline", "test-1", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline stopped", "driver", "memory", "pipeline", "test-2", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&memory.Plugin{},
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

	time.Sleep(time.Second * 1)
	stopCh <- struct{}{}
	wg.Wait()
}

func TestMemoryCreate(t *testing.T) {
	t.Skip("not for the CI")
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Plugin{
		Path:   "memory/.rr-memory-create.yaml",
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
		&memory.Plugin{},
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

	time.Sleep(time.Second * 5)
	t.Run("PushPipeline", pushToPipe("example"))
	stopCh <- struct{}{}
	wg.Wait()
}

func TestMemoryDeclare(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Plugin{
		Path:   "memory/.rr-memory-declare.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	// general
	mockLogger.EXPECT().Debug("RPC plugin started", "address", "tcp://127.0.0.1:6001", "plugins", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Debug("job pushed successfully", "ID", gomock.Any(), "pipeline", "test-3", "driver", "memory", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("job processing started", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("job processed successfully", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)

	mockLogger.EXPECT().Debug("pipeline resumed", "driver", "memory", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline paused", "driver", "memory", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline stopped", "driver", "memory", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&memory.Plugin{},
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

	t.Run("DeclarePipeline", declareMemoryPipe)
	t.Run("ConsumePipeline", consumeMemoryPipe)
	t.Run("PushPipeline", pushToPipe("test-3"))
	time.Sleep(time.Second)
	t.Run("PausePipeline", pausePipelines("test-3"))
	time.Sleep(time.Second)
	t.Run("DestroyPipeline", destroyPipelines("test-3"))

	time.Sleep(time.Second * 5)
	stopCh <- struct{}{}
	wg.Wait()
}

func TestMemoryPauseResume(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Plugin{
		Path:   "memory/.rr-memory-pause-resume.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	// general
	mockLogger.EXPECT().Debug("RPC plugin started", "address", "tcp://127.0.0.1:6001", "plugins", gomock.Any()).Times(1)
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Debug("pipeline active", "driver", "memory", "pipeline", "test-local-2", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline active", "driver", "memory", "pipeline", "test-local", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)

	mockLogger.EXPECT().Debug("pipeline paused", "driver", "memory", "pipeline", "test-local", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline resumed", "driver", "memory", "pipeline", "test-local", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)

	mockLogger.EXPECT().Debug("job pushed successfully", "ID", gomock.Any(), "pipeline", "test-local", "driver", "memory", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("job processing started", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("job processed successfully", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)

	mockLogger.EXPECT().Debug("pipeline stopped", "driver", "memory", "pipeline", "test-local-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline stopped", "driver", "memory", "pipeline", "test-local-2", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline stopped", "driver", "memory", "pipeline", "test-local", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&memory.Plugin{},
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

	t.Run("Pause", pausePipelines("test-local"))
	t.Run("pushToDisabledPipe", pushToDisabledPipe("test-local"))
	t.Run("Resume", resumePipes("test-local"))
	t.Run("pushToEnabledPipe", pushToPipe("test-local"))
	time.Sleep(time.Second * 1)

	stopCh <- struct{}{}
	time.Sleep(time.Second)
	wg.Wait()
}

func TestMemoryJobsError(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Plugin{
		Path:   "memory/.rr-memory-jobs-err.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	// general
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("RPC plugin started", "address", "tcp://127.0.0.1:6001", "plugins", gomock.Any()).Times(1)

	mockLogger.EXPECT().Debug("job pushed successfully", "ID", gomock.Any(), "pipeline", "test-3", "driver", "memory", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("job processing started", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("job processed successfully", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)

	mockLogger.EXPECT().Debug("pipeline resumed", "driver", "memory", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline paused", "driver", "memory", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline stopped", "driver", "memory", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)

	mockLogger.EXPECT().Error("jobs protocol error", "error", "error", "delay", gomock.Any(), "requeue", gomock.Any()).Times(3)

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&memory.Plugin{},
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

	t.Run("DeclarePipeline", declareMemoryPipe)
	t.Run("ConsumePipeline", resumePipes("test-3"))
	t.Run("PushPipeline", pushToPipe("test-3"))
	time.Sleep(time.Second * 25)
	t.Run("PausePipeline", pausePipelines("test-3"))
	time.Sleep(time.Second)
	t.Run("DestroyPipeline", destroyPipelines("test-3"))

	time.Sleep(time.Second * 5)
	stopCh <- struct{}{}
	wg.Wait()
}

func TestMemoryStats(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Plugin{
		Path:   "memory/.rr-memory-declare.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	// general
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug("RPC plugin started", "address", "tcp://127.0.0.1:6001", "plugins", gomock.Any()).Times(1)

	mockLogger.EXPECT().Debug("job pushed successfully", "ID", gomock.Any(), "pipeline", "test-3", "driver", "memory", "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("job processing started", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("job processed successfully", "ID", gomock.Any(), "start", gomock.Any(), "elapsed", gomock.Any()).MinTimes(1)

	mockLogger.EXPECT().Debug("pipeline resumed", "driver", "memory", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(2)
	mockLogger.EXPECT().Debug("pipeline paused", "driver", "memory", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)
	mockLogger.EXPECT().Debug("pipeline stopped", "driver", "memory", "pipeline", "test-3", "start", gomock.Any(), "elapsed", gomock.Any()).Times(1)

	err = cont.RegisterAll(
		cfg,
		&server.Plugin{},
		&rpcPlugin.Plugin{},
		mockLogger,
		&jobs.Plugin{},
		&resetter.Plugin{},
		&informer.Plugin{},
		&memory.Plugin{},
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

	t.Run("DeclarePipeline", declareMemoryPipe)
	t.Run("ConsumePipeline", consumeMemoryPipe)
	t.Run("PushPipeline", pushToPipe("test-3"))
	time.Sleep(time.Second)
	t.Run("PausePipeline", pausePipelines("test-3"))
	time.Sleep(time.Second)

	t.Run("PushPipeline", pushToPipeDelayed("test-3", 5))
	t.Run("PushPipeline", pushToPipe("test-3"))

	time.Sleep(time.Second)
	out := &jobState.State{}
	t.Run("Stats", stats(out))

	assert.Equal(t, out.Pipeline, "test-3")
	assert.Equal(t, out.Driver, "memory")
	assert.Equal(t, out.Queue, "test-3")

	assert.Equal(t, out.Active, int64(1))
	assert.Equal(t, out.Delayed, int64(1))
	assert.Equal(t, out.Reserved, int64(0))

	time.Sleep(time.Second)
	t.Run("ConsumePipeline", consumeMemoryPipe)
	time.Sleep(time.Second * 7)

	out = &jobState.State{}
	t.Run("Stats", stats(out))

	assert.Equal(t, out.Pipeline, "test-3")
	assert.Equal(t, out.Driver, "memory")
	assert.Equal(t, out.Queue, "test-3")

	assert.Equal(t, out.Active, int64(0))
	assert.Equal(t, out.Delayed, int64(0))
	assert.Equal(t, out.Reserved, int64(0))

	t.Run("DestroyEphemeralPipeline", destroyPipelines("test-3"))

	time.Sleep(time.Second * 5)
	stopCh <- struct{}{}
	wg.Wait()
}

func declareMemoryPipe(t *testing.T) {
	conn, err := net.Dial("tcp", "127.0.0.1:6001")
	assert.NoError(t, err)
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))

	pipe := &jobsv1beta.DeclareRequest{Pipeline: map[string]string{
		"driver":   "memory",
		"name":     "test-3",
		"prefetch": "10000",
	}}

	er := &jobsv1beta.Empty{}
	err = client.Call("jobs.Declare", pipe, er)
	assert.NoError(t, err)
}

func consumeMemoryPipe(t *testing.T) {
	conn, err := net.Dial("tcp", "127.0.0.1:6001")
	assert.NoError(t, err)
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))

	pipe := &jobsv1beta.Pipelines{Pipelines: make([]string, 1)}
	pipe.GetPipelines()[0] = "test-3"

	er := &jobsv1beta.Empty{}
	err = client.Call("jobs.Resume", pipe, er)
	assert.NoError(t, err)
}
