package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spiral/errors"
	"github.com/spiral/roadrunner-plugins/v2/config"
	"github.com/spiral/roadrunner-plugins/v2/logger"

	// core imports
	"github.com/spiral/roadrunner/v2/events"
	"github.com/spiral/roadrunner/v2/pool"
	"github.com/spiral/roadrunner/v2/transport"
	"github.com/spiral/roadrunner/v2/transport/pipe"
	"github.com/spiral/roadrunner/v2/transport/socket"
	"github.com/spiral/roadrunner/v2/utils"
	"github.com/spiral/roadrunner/v2/worker"
)

const (
	// PluginName for the server
	PluginName = "server"
	// RrRelay env variable key (internal)
	RrRelay = "RR_RELAY"
	// RrRPC env variable key (internal) if the RPC presents
	RrRPC = "RR_RPC"
)

// Plugin manages worker
type Plugin struct {
	cfg     Config
	log     logger.Logger
	factory transport.Factory
}

// Init application provider.
func (server *Plugin) Init(cfg config.Configurer, log logger.Logger) error {
	const op = errors.Op("server_plugin_init")
	if !cfg.Has(PluginName) {
		return errors.E(op, errors.Disabled)
	}
	err := cfg.Unmarshal(&server.cfg)
	if err != nil {
		return errors.E(op, errors.Init, err)
	}
	server.cfg.InitDefaults()
	server.log = log

	return nil
}

// Name contains service name.
func (server *Plugin) Name() string {
	return PluginName
}

// Available interface implementation
func (server *Plugin) Available() {}

// Serve (Start) server plugin (just a mock here to satisfy interface)
func (server *Plugin) Serve() chan error {
	const op = errors.Op("server_plugin_serve")
	errCh := make(chan error, 1)
	var err error
	server.factory, err = server.initFactory()
	if err != nil {
		errCh <- errors.E(op, err)
		return errCh
	}
	return errCh
}

// Stop used to close chosen in config factory
func (server *Plugin) Stop() error {
	if server.factory == nil {
		return nil
	}

	return server.factory.Close()
}

// CmdFactory provides worker command factory associated with given context.
func (server *Plugin) CmdFactory(env Env) (func() *exec.Cmd, error) {
	const op = errors.Op("server_plugin_cmd_factory")
	var cmdArgs []string

	// create command according to the config
	cmdArgs = append(cmdArgs, strings.Split(server.cfg.Server.Command, " ")...)
	if len(cmdArgs) < 2 {
		return nil, errors.E(op, errors.Str("minimum command should be `<executable> <script>"))
	}

	// try to find a path here
	err := server.scanCommand(cmdArgs)
	if err != nil {
		server.log.Info("scan command", "reason", err)
	}

	return func() *exec.Cmd {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) //nolint:gosec
		utils.IsolateProcess(cmd)

		// if user is not empty, and OS is linux or macos
		// execute php worker from that particular user
		if server.cfg.Server.User != "" {
			err := utils.ExecuteFromUser(cmd, server.cfg.Server.User)
			if err != nil {
				return nil
			}
		}

		cmd.Env = server.setEnv(env)

		return cmd
	}, nil
}

// NewWorker issues new standalone worker.
func (server *Plugin) NewWorker(ctx context.Context, env Env, listeners ...events.Listener) (*worker.Process, error) {
	const op = errors.Op("server_plugin_new_worker")

	list := make([]events.Listener, 0, len(listeners))
	list = append(list, server.collectWorkerEvents)

	spawnCmd, err := server.CmdFactory(env)
	if err != nil {
		return nil, errors.E(op, err)
	}

	w, err := server.factory.SpawnWorkerWithTimeout(ctx, spawnCmd(), list...)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return w, nil
}

// NewWorkerPool issues new worker pool.
func (server *Plugin) NewWorkerPool(ctx context.Context, opt *pool.Config, env Env, listeners ...events.Listener) (pool.Pool, error) {
	const op = errors.Op("server_plugin_new_worker_pool")

	spawnCmd, err := server.CmdFactory(env)
	if err != nil {
		return nil, errors.E(op, err)
	}

	list := make([]events.Listener, 0, 2)
	list = append(list, server.collectPoolEvents, server.collectWorkerEvents)
	if len(listeners) != 0 {
		list = append(list, listeners...)
	}

	p, err := pool.Initialize(ctx, spawnCmd, server.factory, opt, pool.AddListeners(list...))
	if err != nil {
		return nil, errors.E(op, err)
	}

	return p, nil
}

// creates relay and worker factory.
func (server *Plugin) initFactory() (transport.Factory, error) {
	const op = errors.Op("server_plugin_init_factory")
	if server.cfg.Server.Relay == "" || server.cfg.Server.Relay == "pipes" {
		return pipe.NewPipeFactory(), nil
	}

	dsn := strings.Split(server.cfg.Server.Relay, "://")
	if len(dsn) != 2 {
		return nil, errors.E(op, errors.Network, errors.Str("invalid DSN (tcp://:6001, unix://file.sock)"))
	}

	lsn, err := utils.CreateListener(server.cfg.Server.Relay)
	if err != nil {
		return nil, errors.E(op, errors.Network, err)
	}

	switch dsn[0] {
	// sockets group
	case "unix":
		return socket.NewSocketServer(lsn, server.cfg.Server.RelayTimeout), nil
	case "tcp":
		return socket.NewSocketServer(lsn, server.cfg.Server.RelayTimeout), nil
	default:
		return nil, errors.E(op, errors.Network, errors.Str("invalid DSN (tcp://:6001, unix://file.sock)"))
	}
}

func (server *Plugin) setEnv(e Env) []string {
	env := append(os.Environ(), fmt.Sprintf(RrRelay+"=%s", server.cfg.Server.Relay))
	for k, v := range e {
		env = append(env, fmt.Sprintf("%s=%s", strings.ToUpper(k), v))
	}

	if server.cfg.RPC != nil && server.cfg.RPC.Listen != "" {
		env = append(env, fmt.Sprintf("%s=%s", RrRPC, server.cfg.RPC.Listen))
	}

	// set env variables from the config
	if len(server.cfg.Server.Env) > 0 {
		for k, v := range server.cfg.Server.Env {
			env = append(env, fmt.Sprintf("%s=%s", strings.ToUpper(k), v))
		}
	}

	return env
}

func (server *Plugin) collectPoolEvents(event interface{}) {
	if we, ok := event.(events.PoolEvent); ok {
		switch we.Event {
		case events.EventMaxMemory:
			server.log.Warn("worker max memory reached", "pid", we.Payload.(worker.BaseProcess).Pid())
			// debug case
			server.log.Debug("debug", "worker", we.Payload.(worker.BaseProcess))
		case events.EventNoFreeWorkers:
			server.log.Warn("no free workers in the pool, consider increasing `pool.num_workers` property, or `pool.allocate_timeout`")
			// show error only in the debug mode
			server.log.Debug("error", we.Payload.(error).Error())
		case events.EventWorkerProcessExit:
			server.log.Info("worker process exited")
			server.log.Debug("debug", "error", we.Error)
		case events.EventSupervisorError:
			server.log.Error("pool supervisor error, turn on debug logger to see the error")
			// debug
			server.log.Debug("debug", "error", we.Payload.(error).Error())
		case events.EventTTL:
			server.log.Warn("worker TTL reached", "pid", we.Payload.(worker.BaseProcess).Pid())
		case events.EventWorkerConstruct:
			if _, ok := we.Payload.(error); ok {
				server.log.Error("worker construction error", "error", we.Payload.(error).Error())
				return
			}
			server.log.Debug("worker constructed", "pid", we.Payload.(worker.BaseProcess).Pid())
		case events.EventWorkerDestruct:
			server.log.Debug("worker destructed", "pid", we.Payload.(worker.BaseProcess).Pid())
		case events.EventExecTTL:
			server.log.Warn("worker execute timeout reached, consider increasing pool supervisor options")
			// debug
			server.log.Debug("debug", "error", we.Payload.(error).Error())
		case events.EventIdleTTL:
			server.log.Warn("worker idle timeout reached", "pid", we.Payload.(worker.BaseProcess).Pid())
		case events.EventPoolRestart:
			server.log.Warn("requested pool restart")
		}
	}
}

func (server *Plugin) collectWorkerEvents(event interface{}) {
	if we, ok := event.(events.WorkerEvent); ok {
		switch we.Event {
		case events.EventWorkerError:
			switch e := we.Payload.(type) { //nolint:gocritic
			case error:
				if errors.Is(errors.SoftJob, e) {
					// get source error for the softjob error
					server.log.Error(strings.TrimRight(e.(*errors.Error).Err.Error(), " \n\t"))
					return
				}

				// print full error for the other types of errors
				server.log.Error(strings.TrimRight(e.Error(), " \n\t"))
				return
			}
			server.log.Error(strings.TrimRight(we.Payload.(error).Error(), " \n\t"))
		case events.EventWorkerLog:
			server.log.Debug(strings.TrimRight(utils.AsString(we.Payload.([]byte)), " \n\t"))
			// stderr event is INFO level
		case events.EventWorkerStderr:
			server.log.Info(strings.TrimRight(utils.AsString(we.Payload.([]byte)), " \n\t"))
		}
	}
}
