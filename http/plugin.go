package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/spiral/endure"
	"github.com/spiral/errors"
	"github.com/spiral/roadrunner/v2/interfaces/config"
	"github.com/spiral/roadrunner/v2/interfaces/events"
	"github.com/spiral/roadrunner/v2/interfaces/log"
	"github.com/spiral/roadrunner/v2/interfaces/pool"
	"github.com/spiral/roadrunner/v2/interfaces/server"
	"github.com/spiral/roadrunner/v2/interfaces/status"
	"github.com/spiral/roadrunner/v2/interfaces/worker"
	poolImpl "github.com/spiral/roadrunner/v2/pkg/pool"
	"github.com/spiral/roadrunner/v2/plugins/http/attributes"
	"github.com/spiral/roadrunner/v2/util"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sys/cpu"
)

const (
	// PluginName declares plugin name.
	PluginName = "http"

	// EventInitSSL thrown at moment of https initialization. SSL server passed as context.
	EventInitSSL = 750
)

// Middleware interface
type Middleware interface {
	Middleware(f http.Handler) http.HandlerFunc
}

type middleware map[string]Middleware

// Service manages pool, http servers.
type Plugin struct {
	sync.Mutex

	configurer config.Configurer
	server     server.Server
	log        log.Logger

	cfg *Config
	// middlewares to chain
	mdwr middleware
	// WorkerEvent listener to stdout
	listener events.EventListener

	// Pool which attached to all servers
	pool pool.Pool

	// servers RR handler
	handler Handle

	// servers
	http  *http.Server
	https *http.Server
	fcgi  *http.Server
}

// AddListener attaches server event controller.
func (s *Plugin) AddListener(listener events.EventListener) {
	// save listeners for Reset
	s.listener = listener
	s.pool.AddListener(listener)
}

// Init must return configure svc and return true if svc hasStatus enabled. Must return error in case of
// misconfiguration. Services must not be used without proper configuration pushed first.
func (s *Plugin) Init(cfg config.Configurer, log log.Logger, server server.Server) error {
	const op = errors.Op("http Init")
	err := cfg.UnmarshalKey(PluginName, &s.cfg)
	if err != nil {
		return errors.E(op, err)
	}

	err = s.cfg.InitDefaults()
	if err != nil {
		return errors.E(op, err)
	}

	s.configurer = cfg
	s.log = log
	s.mdwr = make(map[string]Middleware)

	if !s.cfg.EnableHTTP() && !s.cfg.EnableTLS() && !s.cfg.EnableFCGI() {
		return errors.E(op, errors.Disabled)
	}

	s.pool, err = server.NewWorkerPool(context.Background(), poolImpl.Config{
		Debug:           s.cfg.Pool.Debug,
		NumWorkers:      s.cfg.Pool.NumWorkers,
		MaxJobs:         s.cfg.Pool.MaxJobs,
		AllocateTimeout: s.cfg.Pool.AllocateTimeout,
		DestroyTimeout:  s.cfg.Pool.DestroyTimeout,
		Supervisor:      s.cfg.Pool.Supervisor,
	}, s.cfg.Env)
	if err != nil {
		return errors.E(op, err)
	}

	s.server = server

	s.AddListener(s.logCallback)

	return nil
}

func (s *Plugin) logCallback(event interface{}) {
	switch ev := event.(type) {
	case ResponseEvent:
		s.log.Debug("http handler response received", "elapsed", ev.Elapsed().String(), "remote address", ev.Request.RemoteAddr)
	case ErrorEvent:
		s.log.Error("error event received", "elapsed", ev.Elapsed().String(), "error", ev.Error)
	case events.WorkerEvent:
		s.log.Debug("worker event received", "event", ev.Event, "worker state", ev.Worker.(worker.BaseProcess).State())
	default:
		fmt.Println(event)
	}
}

// Serve serves the svc.
func (s *Plugin) Serve() chan error {
	s.Lock()
	defer s.Unlock()

	const op = errors.Op("serve http")
	errCh := make(chan error, 2)

	var err error
	s.handler, err = NewHandler(
		s.cfg.MaxRequestSize,
		*s.cfg.Uploads,
		s.cfg.cidrs,
		s.pool,
	)
	if err != nil {
		errCh <- errors.E(op, err)
		return errCh
	}

	if s.listener != nil {
		s.handler.AddListener(s.listener)
	}

	if s.cfg.EnableHTTP() {
		if s.cfg.EnableH2C() {
			s.http = &http.Server{Addr: s.cfg.Address, Handler: h2c.NewHandler(s, &http2.Server{})}
		} else {
			s.http = &http.Server{Addr: s.cfg.Address, Handler: s}
		}
	}

	if s.cfg.EnableTLS() {
		s.https = s.initSSL()
		if s.cfg.SSL.RootCA != "" {
			err = s.appendRootCa()
			if err != nil {
				errCh <- errors.E(op, err)
				return errCh
			}
		}

		if s.cfg.EnableHTTP2() {
			if err := s.initHTTP2(); err != nil {
				errCh <- errors.E(op, err)
				return errCh
			}
		}
	}

	if s.cfg.EnableFCGI() {
		s.fcgi = &http.Server{Handler: s}
	}

	// apply middlewares before starting the server
	if len(s.mdwr) > 0 {
		s.addMiddlewares()
	}

	if s.http != nil {
		go func() {
			httpErr := s.http.ListenAndServe()
			if httpErr != nil && httpErr != http.ErrServerClosed {
				errCh <- errors.E(op, httpErr)
				return
			}
		}()
	}

	if s.https != nil {
		go func() {
			httpErr := s.https.ListenAndServeTLS(
				s.cfg.SSL.Cert,
				s.cfg.SSL.Key,
			)

			if httpErr != nil && httpErr != http.ErrServerClosed {
				errCh <- errors.E(op, httpErr)
				return
			}
		}()
	}

	if s.fcgi != nil {
		go func() {
			httpErr := s.serveFCGI()
			if httpErr != nil && httpErr != http.ErrServerClosed {
				errCh <- errors.E(op, httpErr)
				return
			}
		}()
	}

	return errCh
}

// Stop stops the http.
func (s *Plugin) Stop() error {
	s.Lock()
	defer s.Unlock()

	var err error
	if s.fcgi != nil {
		err = s.fcgi.Shutdown(context.Background())
		if err != nil && err != http.ErrServerClosed {
			s.log.Error("error shutting down the fcgi server", "error", err)
			// write error and try to stop other transport
			err = multierror.Append(err)
		}
	}

	if s.https != nil {
		err = s.https.Shutdown(context.Background())
		if err != nil && err != http.ErrServerClosed {
			s.log.Error("error shutting down the https server", "error", err)
			// write error and try to stop other transport
			err = multierror.Append(err)
		}
	}

	if s.http != nil {
		err = s.http.Shutdown(context.Background())
		if err != nil && err != http.ErrServerClosed {
			s.log.Error("error shutting down the http server", "error", err)
			// write error and try to stop other transport
			err = multierror.Append(err)
		}
	}

	return err
}

// ServeHTTP handles connection using set of middleware and pool PSR-7 server.
func (s *Plugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if headerContainsUpgrade(r, s) {
		http.Error(w, "server does not support upgrade header", http.StatusInternalServerError)
		return
	}

	if s.redirect(w, r) {
		return
	}

	if s.https != nil && r.TLS != nil {
		w.Header().Add("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	}

	r = attributes.Init(r)
	// protect the case, when user send Reset and we are replacing handler with pool
	s.Lock()
	s.handler.ServeHTTP(w, r)
	s.Unlock()
}

// Server returns associated pool workers
func (s *Plugin) Workers() []worker.BaseProcess {
	return s.pool.Workers()
}

func (s *Plugin) Name() string {
	return PluginName
}

func (s *Plugin) Reset() error {
	s.Lock()
	defer s.Unlock()
	const op = errors.Op("http reset")
	s.log.Info("HTTP plugin got restart request. Restarting...")
	s.pool.Destroy(context.Background())

	// re-read the config
	err := s.configurer.UnmarshalKey(PluginName, &s.cfg)
	if err != nil {
		return errors.E(op, err)
	}

	s.pool, err = s.server.NewWorkerPool(context.Background(), poolImpl.Config{
		Debug:           s.cfg.Pool.Debug,
		NumWorkers:      s.cfg.Pool.NumWorkers,
		MaxJobs:         s.cfg.Pool.MaxJobs,
		AllocateTimeout: s.cfg.Pool.AllocateTimeout,
		DestroyTimeout:  s.cfg.Pool.DestroyTimeout,
		Supervisor:      s.cfg.Pool.Supervisor,
	}, s.cfg.Env)
	if err != nil {
		return errors.E(op, err)
	}

	s.log.Info("HTTP workers Pool successfully restarted")
	s.handler, err = NewHandler(
		s.cfg.MaxRequestSize,
		*s.cfg.Uploads,
		s.cfg.cidrs,
		s.pool,
	)
	if err != nil {
		return errors.E(op, err)
	}

	// restore original listeners
	s.pool.AddListener(s.listener)
	s.log.Info("HTTP listeners successfully re-added")

	s.log.Info("HTTP plugin successfully restarted")
	return nil
}

func (s *Plugin) Collects() []interface{} {
	return []interface{}{
		s.AddMiddleware,
	}
}

func (s *Plugin) AddMiddleware(name endure.Named, m Middleware) {
	s.mdwr[name.Name()] = m
}

// Status return status of the particular plugin
func (s *Plugin) Status() status.Status {
	workers := s.Workers()
	for i := 0; i < len(workers); i++ {
		if workers[i].State().IsActive() {
			return status.Status{
				Code: http.StatusOK,
			}
		}
	}
	// if there are no workers, threat this as error
	return status.Status{
		Code: http.StatusInternalServerError,
	}
}

func (s *Plugin) redirect(w http.ResponseWriter, r *http.Request) bool {
	if s.https != nil && r.TLS == nil && s.cfg.SSL.Redirect {
		target := &url.URL{
			Scheme:   "https",
			Host:     s.tlsAddr(r.Host, false),
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
		}

		http.Redirect(w, r, target.String(), http.StatusTemporaryRedirect)
		return true
	}
	return false
}

func headerContainsUpgrade(r *http.Request, s *Plugin) bool {
	if _, ok := r.Header["Upgrade"]; ok {
		// https://golang.org/pkg/net/http/#Hijacker
		s.log.Error("server does not support Upgrade header")
		return true
	}
	return false
}

// append RootCA to the https server TLS config
func (s *Plugin) appendRootCa() error {
	const op = errors.Op("append root CA")
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil
	}
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	CA, err := ioutil.ReadFile(s.cfg.SSL.RootCA)
	if err != nil {
		return err
	}

	// should append our CA cert
	ok := rootCAs.AppendCertsFromPEM(CA)
	if !ok {
		return errors.E(op, errors.Str("could not append Certs from PEM"))
	}
	// disable "G402 (CWE-295): TLS MinVersion too low. (Confidence: HIGH, Severity: HIGH)"
	// #nosec G402
	cfg := &tls.Config{
		InsecureSkipVerify: false,
		RootCAs:            rootCAs,
	}
	s.http.TLSConfig = cfg

	return nil
}

// Init https server
func (s *Plugin) initSSL() *http.Server {
	var topCipherSuites []uint16
	var defaultCipherSuitesTLS13 []uint16

	hasGCMAsmAMD64 := cpu.X86.HasAES && cpu.X86.HasPCLMULQDQ
	hasGCMAsmARM64 := cpu.ARM64.HasAES && cpu.ARM64.HasPMULL
	// Keep in sync with crypto/aes/cipher_s390x.go.
	hasGCMAsmS390X := cpu.S390X.HasAES && cpu.S390X.HasAESCBC && cpu.S390X.HasAESCTR && (cpu.S390X.HasGHASH || cpu.S390X.HasAESGCM)

	hasGCMAsm := hasGCMAsmAMD64 || hasGCMAsmARM64 || hasGCMAsmS390X

	if hasGCMAsm {
		// If AES-GCM hardware is provided then prioritise AES-GCM
		// cipher suites.
		topCipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		}
		defaultCipherSuitesTLS13 = []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
		}
	} else {
		// Without AES-GCM hardware, we put the ChaCha20-Poly1305
		// cipher suites first.
		topCipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		}
		defaultCipherSuitesTLS13 = []uint16{
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
		}
	}

	DefaultCipherSuites := make([]uint16, 0, 22)
	DefaultCipherSuites = append(DefaultCipherSuites, topCipherSuites...)
	DefaultCipherSuites = append(DefaultCipherSuites, defaultCipherSuitesTLS13...)

	server := &http.Server{
		Addr:    s.tlsAddr(s.cfg.Address, true),
		Handler: s,
		TLSConfig: &tls.Config{
			CurvePreferences: []tls.CurveID{
				tls.CurveP256,
				tls.CurveP384,
				tls.CurveP521,
				tls.X25519,
			},
			CipherSuites:             DefaultCipherSuites,
			MinVersion:               tls.VersionTLS12,
			PreferServerCipherSuites: true,
		},
	}

	return server
}

// init http/2 server
func (s *Plugin) initHTTP2() error {
	return http2.ConfigureServer(s.https, &http2.Server{
		MaxConcurrentStreams: s.cfg.HTTP2.MaxConcurrentStreams,
	})
}

// serveFCGI starts FastCGI server.
func (s *Plugin) serveFCGI() error {
	l, err := util.CreateListener(s.cfg.FCGI.Address)
	if err != nil {
		return err
	}

	err = fcgi.Serve(l, s.fcgi.Handler)
	if err != nil {
		return err
	}

	return nil
}

// tlsAddr replaces listen or host port with port configured by SSL config.
func (s *Plugin) tlsAddr(host string, forcePort bool) string {
	// remove current forcePort first
	host = strings.Split(host, ":")[0]

	if forcePort || s.cfg.SSL.Port != 443 {
		host = fmt.Sprintf("%s:%v", host, s.cfg.SSL.Port)
	}

	return host
}

func (s *Plugin) addMiddlewares() {
	if s.http != nil {
		applyMiddlewares(s.http, s.mdwr, s.cfg.Middleware, s.log)
	}
	if s.https != nil {
		applyMiddlewares(s.https, s.mdwr, s.cfg.Middleware, s.log)
	}

	if s.fcgi != nil {
		applyMiddlewares(s.fcgi, s.mdwr, s.cfg.Middleware, s.log)
	}
}

func applyMiddlewares(server *http.Server, middlewares map[string]Middleware, order []string, log log.Logger) {
	for i := 0; i < len(order); i++ {
		if mdwr, ok := middlewares[order[i]]; ok {
			server.Handler = mdwr.Middleware(server.Handler)
		} else {
			log.Warn("requested middleware does not exist", "requested", order[i])
		}
	}
}
