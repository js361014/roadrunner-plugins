package tests

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/spiral/endure"
	"github.com/spiral/roadrunner/v2/mocks"
	"github.com/spiral/roadrunner/v2/plugins/config"
	"github.com/spiral/roadrunner/v2/plugins/gzip"
	httpPlugin "github.com/spiral/roadrunner/v2/plugins/http"
	"github.com/spiral/roadrunner/v2/plugins/logger"
	"github.com/spiral/roadrunner/v2/plugins/server"
	"github.com/spiral/roadrunner/v2/plugins/static"
	"github.com/stretchr/testify/assert"
)

func TestStaticPlugin(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-http-static.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&gzip.Gzip{},
		&static.Plugin{},
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

	time.Sleep(time.Second)
	t.Run("ServeSample", serveStaticSample)
	t.Run("StaticNotForbid", staticNotForbid)
	t.Run("StaticHeaders", staticHeaders)
	wg.Wait()
}

func staticHeaders(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:21603/client.php", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Header.Get("Output") != "output-header" {
		t.Fatal("can't find output header in response")
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, all("../../../tests/client.php"), string(b))
	assert.Equal(t, all("../../../tests/client.php"), string(b))
}

func staticNotForbid(t *testing.T) {
	b, r, err := get("http://localhost:21603/client.php")
	assert.NoError(t, err)
	assert.Equal(t, all("../../../tests/client.php"), b)
	assert.Equal(t, all("../../../tests/client.php"), b)
	_ = r.Body.Close()
}

func serveStaticSample(t *testing.T) {
	b, r, err := get("http://localhost:21603/sample.txt")
	assert.NoError(t, err)
	assert.Equal(t, "sample", b)
	_ = r.Body.Close()
}

func TestStaticDisabled(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-http-static-disabled.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&gzip.Gzip{},
		&static.Plugin{},
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

	time.Sleep(time.Second)
	t.Run("StaticDisabled", staticDisabled)
	wg.Wait()
}

func staticDisabled(t *testing.T) {
	_, r, err := get("http://localhost:21234/sample.txt")
	assert.Error(t, err)
	assert.Nil(t, r)
}

func TestStaticFilesDisabled(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-http-static-files-disable.yaml",
		Prefix: "rr",
	}

	err = cont.RegisterAll(
		cfg,
		&logger.ZapLogger{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&gzip.Gzip{},
		&static.Plugin{},
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

	time.Sleep(time.Second)
	t.Run("StaticFilesDisabled", staticFilesDisabled)
	wg.Wait()
}

func staticFilesDisabled(t *testing.T) {
	b, r, err := get("http://localhost:45877/client.php?hello=world")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "WORLD", b)
	_ = r.Body.Close()
}

func TestStaticFilesForbid(t *testing.T) {
	cont, err := endure.NewContainer(nil, endure.SetLogLevel(endure.ErrorLevel))
	assert.NoError(t, err)

	cfg := &config.Viper{
		Path:   "configs/.rr-http-static-files.yaml",
		Prefix: "rr",
	}

	controller := gomock.NewController(t)
	mockLogger := mocks.NewMockLogger(controller)

	mockLogger.EXPECT().Debug("http handler response received", "elapsed", gomock.Any(), "remote address", "127.0.0.1").AnyTimes()
	mockLogger.EXPECT().Error("file open error", "error", gomock.Any()).AnyTimes()

	err = cont.RegisterAll(
		cfg,
		mockLogger,
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&gzip.Gzip{},
		&static.Plugin{},
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

	time.Sleep(time.Second)
	t.Run("StaticTestFilesDir", staticTestFilesDir)
	t.Run("StaticNotFound", staticNotFound)
	t.Run("StaticFilesForbid", staticFilesForbid)
	t.Run("StaticFilesAlways", staticFilesAlways)
	wg.Wait()
}

func staticTestFilesDir(t *testing.T) {
	b, r, err := get("http://localhost:34653/http?hello=world")
	assert.NoError(t, err)
	assert.Equal(t, "WORLD", b)
	_ = r.Body.Close()
}

func staticNotFound(t *testing.T) {
	b, _, _ := get("http://localhost:34653/client.XXX?hello=world")
	assert.Equal(t, "WORLD", b)
}

func staticFilesAlways(t *testing.T) {
	_, r, err := get("http://localhost:34653/favicon.ico")
	assert.NoError(t, err)
	assert.Equal(t, 404, r.StatusCode)
	_ = r.Body.Close()
}

func staticFilesForbid(t *testing.T) {
	b, r, err := get("http://localhost:34653/client.php?hello=world")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "WORLD", b)
	_ = r.Body.Close()
}

// HELPERS
func get(url string) (string, *http.Response, error) {
	r, err := http.Get(url)
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

func all(fn string) string {
	f, _ := os.Open(fn)

	b := new(bytes.Buffer)
	_, err := io.Copy(b, f)
	if err != nil {
		return ""
	}

	err = f.Close()
	if err != nil {
		return ""
	}

	return b.String()
}
