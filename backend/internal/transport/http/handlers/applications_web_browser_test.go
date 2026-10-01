package httphandlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

// This opt-in check uses real Chromium cookie, Fetch Metadata, CORS, navigation,
// and WebSocket behavior. Run with FUTRX_BROWSER_TEST=1 and Playwright installed.
func TestApplicationWebBrowserIsolation(t *testing.T) {
	if os.Getenv("FUTRX_BROWSER_TEST") != "1" {
		t.Skip("set FUTRX_BROWSER_TEST=1 to run the Playwright browser check")
	}
	gateway := httptest.NewUnstartedServer(nil)
	defer gateway.Close()
	_, port, err := net.SplitHostPort(gateway.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	base := "https://remote.test:" + port
	appBase := "https://" + webTestHost + ":" + port
	f := newWebFixture(t, base)
	var reads, writes, sockets atomic.Int32
	var leaked atomic.Bool
	upgrader := httptransport.NewUpgrader()
	echo := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		kind, data, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(kind, data)
		}
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			leaked.Store(true)
		}
		if r.URL.Path == "/socket" {
			echo(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<!doctype html><title>Application fixture</title><p>APPLICATION</p>")
	}))
	defer upstream.Close()
	f.apps.webTransport = &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(upstream.URL, "http://"))
	}}
	platform := http.NewServeMux()
	f.apps.RegisterRoutes(platform)
	NewAuthHandler(f.auth, serviceauth.NewAccessVerifier(f.auth, nil)).RegisterRoutes(platform)
	platform.HandleFunc("/api/private", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			reads.Add(1)
		} else {
			writes.Add(1)
		}
		fmt.Fprint(w, "PLATFORM_SECRET")
	})
	platform.HandleFunc("/ws/private", func(w http.ResponseWriter, r *http.Request) { sockets.Add(1); echo(w, r) })
	platform.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<!doctype html><title>Remote fixture</title><p>REMOTE</p>")
	})
	gateway.Config.Handler = httpmiddleware.NewBrowserProtection(base).Wrap(f.apps.WebHandler(httpmiddleware.NewAuth(f.auth).Wrap(platform)))
	gateway.StartTLS()
	input, err := json.Marshal(map[string]string{
		"base": base, "app": appBase, "launch": base + "/apps/project/editor/",
		"cookie": f.cookie(t, "member@example.test").Value,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "testdata/application_web_browser.mjs")
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser check: %v\n%s", err, output)
	}
	t.Log(strings.TrimSpace(string(output)))
	if reads.Load() != 1 || writes.Load() != 0 || sockets.Load() != 1 || leaked.Load() {
		t.Fatalf("reads=%d writes=%d sockets=%d leaked=%v", reads.Load(), writes.Load(), sockets.Load(), leaked.Load())
	}
}
