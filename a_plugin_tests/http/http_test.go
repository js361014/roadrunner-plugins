package http

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/spiral/endure"
	goridgeRpc "github.com/spiral/goridge/v3/pkg/rpc"
	"github.com/spiral/roadrunner-plugins/a_plugin_tests/mocks"
	"github.com/spiral/roadrunner-plugins/config"
	httpPlugin "github.com/spiral/roadrunner-plugins/http"
	"github.com/spiral/roadrunner-plugins/informer"
	"github.com/spiral/roadrunner-plugins/logger"
	"github.com/spiral/roadrunner-plugins/resetter"
	"github.com/spiral/roadrunner/v2/interfaces/events"
	"github.com/yookoala/gofast"

	rpcPlugin "github.com/spiral/roadrunner-plugins/rpc"
	"github.com/spiral/roadrunner-plugins/server"
	"github.com/stretchr/testify/assert"
)

var sslClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		},
	},
}

func TestHTTPInit(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	rIn := makeConfig("6001", "15395", "7921", "8892", "false", "false", "php ../../tests/http/client.php echo pipes")
	cfg := &config.Viper{
		ReadInCfg: rIn,
		Type:      "yaml",
	}

	err = cont.RegisterAll(
		cfg,
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	tt := time.NewTimer(time.Second * 5)

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
			case <-tt.C:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	wg.Wait()
}

func TestHTTPInformerReset(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-resetter.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&informer.Plugin{},
		&resetter.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		tt := time.NewTimer(time.Second * 10)
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
			case <-tt.C:
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
	t.Run("HTTPInformerTest", informerTest)
	t.Run("HTTPEchoTestBefore", echoHTTP)
	t.Run("HTTPResetTest", resetTest)
	t.Run("HTTPEchoTestAfter", echoHTTP)

	wg.Wait()
}

func echoHTTP(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:10084?hello=world", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	b, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, 201, r.StatusCode)
	assert.Equal(t, "WORLD", string(b))

	err = r.Body.Close()
	assert.NoError(t, err)
}

func resetTest(t *testing.T) {
	conn, err := net.Dial("tcp", "127.0.0.1:6001")
	assert.NoError(t, err)
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	// WorkerList contains list of workers.

	var ret bool
	err = client.Call("resetter.Reset", "http", &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
	ret = false

	var services []string
	err = client.Call("resetter.List", nil, &services)
	assert.NoError(t, err)
	if services[0] != "http" {
		t.Fatal("no enough services")
	}
}

func informerTest(t *testing.T) {
	conn, err := net.Dial("tcp", "127.0.0.1:6001")
	assert.NoError(t, err)
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	// WorkerList contains list of workers.
	list := struct {
		// Workers is list of workers.
		Workers []informer.ProcessState `json:"workers"`
	}{}

	err = client.Call("informer.Workers", "http", &list)
	assert.NoError(t, err)
	assert.Len(t, list.Workers, 2)
}

func TestSSL(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-ssl.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	tt := time.NewTimer(time.Second * 10)

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
			case <-tt.C:
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
	t.Run("SSLEcho", sslEcho)
	t.Run("SSLNoRedirect", sslNoRedirect)
	t.Run("fCGIecho", fcgiEcho)
	wg.Wait()
}

func sslNoRedirect(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:8085?hello=world", nil)
	assert.NoError(t, err)

	r, err := sslClient.Do(req)
	assert.NoError(t, err)

	assert.Nil(t, r.TLS)

	b, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, 201, r.StatusCode)
	assert.Equal(t, "WORLD", string(b))

	err2 := r.Body.Close()
	if err2 != nil {
		t.Errorf("fail to close the Body: error %v", err2)
	}
}

func sslEcho(t *testing.T) {
	req, err := http.NewRequest("GET", "https://localhost:8893?hello=world", nil)
	assert.NoError(t, err)

	r, err := sslClient.Do(req)
	assert.NoError(t, err)

	b, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, 201, r.StatusCode)
	assert.Equal(t, "WORLD", string(b))

	err2 := r.Body.Close()
	if err2 != nil {
		t.Errorf("fail to close the Body: error %v", err2)
	}
}

func fcgiEcho(t *testing.T) {
	fcgiConnFactory := gofast.SimpleConnFactory("tcp", "0.0.0.0:16920")

	fcgiHandler := gofast.NewHandler(
		gofast.BasicParamsMap(gofast.BasicSession),
		gofast.SimpleClientFactory(fcgiConnFactory, 0),
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://site.local/?hello=world", nil)
	fcgiHandler.ServeHTTP(w, req)

	body, err := ioutil.ReadAll(w.Result().Body) //nolint:bodyclose

	defer func() {
		_ = w.Result().Body.Close()
		w.Body.Reset()
	}()

	assert.NoError(t, err)
	assert.Equal(t, 201, w.Result().StatusCode) //nolint:bodyclose
	assert.Equal(t, "WORLD", string(body))
}

func TestSSLRedirect(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-ssl-redirect.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	tt := time.NewTimer(time.Second * 10)
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
			case <-tt.C:
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
	t.Run("SSLRedirect", sslRedirect)
	wg.Wait()
}

func sslRedirect(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:8087?hello=world", nil)
	assert.NoError(t, err)

	r, err := sslClient.Do(req)
	assert.NoError(t, err)
	assert.NotNil(t, r.TLS)

	b, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, 201, r.StatusCode)
	assert.Equal(t, "WORLD", string(b))

	err2 := r.Body.Close()
	if err2 != nil {
		t.Errorf("fail to close the Body: error %v", err2)
	}
}

func TestSSLPushPipes(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-ssl-push.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	tt := time.NewTimer(time.Second * 10)
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
			case <-tt.C:
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
	t.Run("SSLPush", sslPush)
	wg.Wait()
}

func sslPush(t *testing.T) {
	req, err := http.NewRequest("GET", "https://localhost:8894?hello=world", nil)
	assert.NoError(t, err)

	r, err := sslClient.Do(req)
	assert.NoError(t, err)

	assert.NotNil(t, r.TLS)

	b, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	assert.Equal(t, "", r.Header.Get("Http2-Push"))

	assert.NoError(t, err)
	assert.Equal(t, 201, r.StatusCode)
	assert.Equal(t, "WORLD", string(b))

	err2 := r.Body.Close()
	if err2 != nil {
		t.Errorf("fail to close the Body: error %v", err2)
	}
}

func TestFastCGI_RequestUri(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-fcgi-reqUri.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	tt := time.NewTimer(time.Second * 10)

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
			case <-tt.C:
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
	t.Run("FastCGIServiceRequestUri", fcgiReqURI)
	wg.Wait()
}

func fcgiReqURI(t *testing.T) {
	time.Sleep(time.Second * 2)
	fcgiConnFactory := gofast.SimpleConnFactory("tcp", "127.0.0.1:6921")

	fcgiHandler := gofast.NewHandler(
		gofast.BasicParamsMap(gofast.BasicSession),
		gofast.SimpleClientFactory(fcgiConnFactory, 0),
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://site.local/hello-world", nil)
	fcgiHandler.ServeHTTP(w, req)

	body, err := ioutil.ReadAll(w.Result().Body) //nolint:bodyclose
	assert.NoError(t, err)
	assert.Equal(t, 200, w.Result().StatusCode) //nolint:bodyclose
	assert.Equal(t, "http://site.local/hello-world", string(body))
}

func TestH2CUpgrade(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-h2c.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	tt := time.NewTimer(time.Second * 10)

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
			case <-tt.C:
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
	t.Run("H2cUpgrade", h2cUpgrade)
	wg.Wait()
}

func h2cUpgrade(t *testing.T) {
	req, err := http.NewRequest("PRI", "http://localhost:8083?hello=world", nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Upgrade", "h2c")
	req.Header.Add("Connection", "HTTP2-Settings")
	req.Header.Add("HTTP2-Settings", "")

	r, err2 := http.DefaultClient.Do(req)
	if err2 != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "101 Switching Protocols", r.Status)

	err3 := r.Body.Close()
	if err3 != nil {
		t.Fatal(err)
	}
}

func TestH2C(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-h2c.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	tt := time.NewTimer(time.Second * 10)

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
			case <-tt.C:
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
	t.Run("H2c", h2c)
	wg.Wait()
}

func h2c(t *testing.T) {
	req, err := http.NewRequest("PRI", "http://localhost:8083?hello=world", nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Connection", "HTTP2-Settings")
	req.Header.Add("HTTP2-Settings", "")

	r, err2 := http.DefaultClient.Do(req)
	if err2 != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "201 Created", r.Status)

	err3 := r.Body.Close()
	if err3 != nil {
		t.Fatal(err)
	}
}

func TestHttpMiddleware(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-http.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&rpcPlugin.Plugin{},
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&PluginMiddleware{},
		&PluginMiddleware2{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	tt := time.NewTimer(time.Second * 20)

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
			case <-tt.C:
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
	t.Run("MiddlewareTest", middleware)
	wg.Wait()
}

func middleware(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:18903?hello=world", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)

	b, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	assert.Equal(t, 201, r.StatusCode)
	assert.Equal(t, "WORLD", string(b))

	err = r.Body.Close()
	assert.NoError(t, err)

	req, err = http.NewRequest("GET", "http://localhost:18903/halt", nil)
	assert.NoError(t, err)

	r, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	b, err = ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	assert.Equal(t, 500, r.StatusCode)
	assert.Equal(t, "halted", string(b))

	err = r.Body.Close()
	assert.NoError(t, err)
}

func TestHttpEchoErr(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.DebugLevel))
	assert.NoError(t, err)

	rIn := `
rpc:
  listen: tcp://127.0.0.1:6001
  disabled: false

server:
  command: "php ../../tests/http/client.php echoerr pipes"
  user: ""
  group: ""
  env:
    "RR_HTTP": "true"
  relay: "pipes"
  relayTimeout: "20s"

http:
  debug: true
  address: 127.0.0.1:34999
  maxRequestSize: 1024
  middleware: [ "pluginMiddleware", "pluginMiddleware2" ]
  uploads:
    forbid: [ "" ]
  trustedSubnets: [ "10.0.0.0/8", "127.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128", "fc00::/7", "fe80::/10" ]
  pool:
    numWorkers: 2
    maxJobs: 0
    allocateTimeout: 60s
    destroyTimeout: 60s
logs:
  mode: development
  level: error
`

	cfg := &config.Viper{
		Path:      "",
		Prefix:    "",
		Type:      "yaml",
		ReadInCfg: []byte(rIn),
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	mockLogger.EXPECT().Info("worker destructed", "pid", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Info("worker constructed", "pid", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("http handler response received", "elapsed", gomock.Any(), "remote address", "127.0.0.1").MinTimes(1)
	mockLogger.EXPECT().Info("WORLD", "pid", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Debug("worker event received", "event", events.EventWorkerLog, "worker state", gomock.Any()).MinTimes(1)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes() // placeholder for the workerlogerror

	err = cont.RegisterAll(
		cfg,
		mockLogger,
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&PluginMiddleware{},
		&PluginMiddleware2{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	tt := time.NewTimer(time.Second * 10)

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
			case <-tt.C:
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
	t.Run("HttpEchoError", echoError)
	wg.Wait()
}

func echoError(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:34999?hello=world", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)

	b, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	assert.Equal(t, 201, r.StatusCode)
	assert.Equal(t, "WORLD", string(b))
	err = r.Body.Close()
	assert.NoError(t, err)
}

func TestHttpEnvVariables(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-env.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&PluginMiddleware{},
		&PluginMiddleware2{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	tt := time.NewTimer(time.Second * 10)

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
			case <-tt.C:
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
	t.Run("EnvVariablesTest", envVarsTest)
	wg.Wait()
}

func envVarsTest(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:12084", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)

	b, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	assert.Equal(t, 200, r.StatusCode)
	assert.Equal(t, "ENV_VALUE", string(b))

	err = r.Body.Close()
	assert.NoError(t, err)
}

func TestHttpBrokenPipes(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-broken-pipes.yaml",
		Prefix: "rr",
		Type:   "yaml",
	}

	err = cont.RegisterAll(
		cfg,
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&PluginMiddleware{},
		&PluginMiddleware2{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	assert.Error(t, err)

	_, err = cont.Serve()
	assert.Error(t, err)
}

func get(url string) (string, *http.Response, error) {
	r, err := http.Get(url) //nolint:gosec
	if err != nil {
		return "", nil, err
	}
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", nil, err
	}
	defer func() {
		_ = r.Body.Close()
	}()
	return string(b), r, err
}

// get request and return body
func getHeader(url string, h map[string]string) (string, *http.Response, error) {
	req, err := http.NewRequest("GET", url, bytes.NewBuffer(nil))
	if err != nil {
		return "", nil, err
	}

	for k, v := range h {
		req.Header.Set(k, v)
	}

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", nil, err
	}

	err = r.Body.Close()
	if err != nil {
		return "", nil, err
	}
	return string(b), r, err
}

func makeConfig(rpcPort, httpPort, fcgiPort, sslPort, redirect, http2Enabled, command string) []byte {
	return []byte(fmt.Sprintf(`
rpc:
  listen: tcp://127.0.0.1:%s
  disabled: false

server:
  command: "%s"
  user: ""
  group: ""
  env:
    "RR_HTTP": "true"
  relay: "pipes"
  relayTimeout: "20s"

http:
  address: 127.0.0.1:%s
  maxRequestSize: 1024
  middleware: [ "" ]
  uploads:
    forbid: [ ".php", ".exe", ".bat" ]
  trustedSubnets: [ "10.0.0.0/8", "127.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "::1/128", "fc00::/7", "fe80::/10" ]
  pool:
    numWorkers: 2
    maxJobs: 0
    allocateTimeout: 60s
    destroyTimeout: 60s

  ssl:
    port: %s
    redirect: %s
    cert: fixtures/server.crt
    key: fixtures/server.key
  #    rootCa: root.crt
  fcgi:
    address: tcp://0.0.0.0:%s
  http2:
    enabled: %s
    h2c: false
    maxConcurrentStreams: 128
logs:
  mode: development
  level: error
`, rpcPort, command, httpPort, sslPort, redirect, fcgiPort, http2Enabled))
}
