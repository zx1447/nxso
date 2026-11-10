package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/urfave/cli/v2"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/cloudflare/cloudflared/cmd/cloudflared/cliutil"
	"github.com/cloudflare/cloudflared/cmd/cloudflared/tunnel"
	"github.com/cloudflare/cloudflared/cmd/cloudflared/updater"
	"github.com/cloudflare/cloudflared/config"
	"github.com/cloudflare/cloudflared/metrics"
)

type tunnelRun struct {
	cancel    context.CancelFunc
	shutdownC chan struct{}
	stopOnce  sync.Once
	stopped   atomic.Bool
	done      chan struct{}
}

func (r *tunnelRun) stop() {
	r.stopped.Store(true)
	r.stopOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
		if r.shutdownC != nil {
			func() {
				defer func() { _ = recover() }()
				close(r.shutdownC)
			}()
		}
	})
}

var (
	startMu         sync.Mutex
	stateMu         sync.Mutex
	activeRun       *tunnelRun
	lastErr         string
	activeTunnelURL string
	usedYAMLConfig  atomic.Bool
)

func init() {
	cli.OsExiter = func(int) {}
	cli.ErrWriter = io.Discard
}

func setLastError(msg string) {
	stateMu.Lock()
	lastErr = msg
	stateMu.Unlock()
}

func allocLoopbackAddr() string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "127.0.0.1:0"
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

type parsedPayload struct {
	mode     string // "quick", "token", "yaml"
	url      string
	protocol string
	token    string
	yamlBody string
}

func parsePayload(raw string) (*parsedPayload, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, fmt.Errorf("empty payload")
	}

	isYaml := strings.Contains(s, "ingress:") || strings.Contains(s, "tunnel:") || strings.Contains(s, "credentials-file:")
	if !isYaml && (strings.Contains(s, "\n") || strings.HasPrefix(s, "url:")) {
		p := &parsedPayload{mode: "quick", protocol: "auto"}
		hasYAMLOnlyKey := false
		for _, line := range strings.Split(s, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if strings.HasPrefix(trimmed, "url:") {
				p.url = strings.TrimSpace(strings.TrimPrefix(trimmed, "url:"))
			} else if strings.HasPrefix(trimmed, "protocol:") {
				val := strings.TrimSpace(strings.TrimPrefix(trimmed, "protocol:"))
				if val != "" {
					p.protocol = val
				}
			} else {
				hasYAMLOnlyKey = true
			}
		}
		if !hasYAMLOnlyKey {
			if p.url == "" {
				return nil, fmt.Errorf("missing url in quick tunnel payload")
			}
			return p, nil
		}
		isYaml = true
	}

	if isYaml || strings.Contains(s, "\n") {
		return &parsedPayload{mode: "yaml", yamlBody: s}, nil
	}

	return &parsedPayload{mode: "token", token: s}, nil
}

func pollQuickTunnelURL(ctx context.Context, metricsAddr string) {
	if metricsAddr == "" || metricsAddr == "127.0.0.1:0" {
		return
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	endpoint := "http://" + metricsAddr + "/quicktunnel"
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if readErr != nil || resp.StatusCode != http.StatusOK {
				continue
			}
			var parsed struct {
				Hostname string `json:"hostname"`
			}
			if json.Unmarshal(body, &parsed) == nil {
				host := strings.TrimSpace(parsed.Hostname)
				host = strings.TrimPrefix(host, "https://")
				host = strings.TrimPrefix(host, "http://")
				if host != "" {
					stateMu.Lock()
					activeTunnelURL = host
					stateMu.Unlock()
					return
				}
			}
		}
	}
}

//export StartCloudflared
func StartCloudflared(payload *C.char) (rc C.int) {
	defer func() {
		if r := recover(); r != nil {
			setLastError(fmt.Sprintf("panic: %v", r))
			rc = -4
		}
	}()

	var rawPayload string
	if payload != nil {
		rawPayload = C.GoString(payload)
	}
	parsed, err := parsePayload(rawPayload)
	if err != nil {
		setLastError(err.Error())
		return -1
	}

	startMu.Lock()
	defer startMu.Unlock()

	stateMu.Lock()
	prev := activeRun
	stateMu.Unlock()
	if prev != nil {
		prev.stop()
		<-prev.done
	}

	_ = os.Setenv("QUIC_GO_DISABLE_ECN", "1")
	_ = os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")
	_, _ = maxprocs.Set()

	cli.OsExiter = func(int) {}
	cli.ErrWriter = io.Discard

	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	prometheus.DefaultGatherer = reg

	*config.GetConfiguration() = config.Configuration{}

	metricsAddr := "127.0.0.1:0"
	if parsed.mode == "quick" {
		metricsAddr = allocLoopbackAddr()
	}

	var tempYAML string
	var emptyConfigReset string
	if parsed.mode != "yaml" && usedYAMLConfig.Load() {
		if ef, efErr := os.CreateTemp("", ".cf-reset-*.yml"); efErr == nil {
			emptyConfigReset = ef.Name()
			_ = ef.Close()
			defer func() { _ = os.Remove(emptyConfigReset) }()
			usedYAMLConfig.Store(false)
		}
	}
	var args []string
	switch parsed.mode {
	case "quick":
		args = []string{
			"cloudflared",
			"--no-autoupdate",
			"--no-prechecks",
			"--loglevel", "fatal",
			"--transport-loglevel", "fatal",
			"--metrics", metricsAddr,
		}
		if emptyConfigReset != "" {
			args = append(args, "--config", emptyConfigReset)
		}
		args = append(args, "--url", parsed.url, "--protocol", parsed.protocol)
	case "yaml":
		f, tmpErr := os.CreateTemp("", ".cf-*.yml")
		if tmpErr != nil {
			setLastError(tmpErr.Error())
			return -2
		}
		tempYAML = f.Name()
		_, writeErr := f.WriteString(parsed.yamlBody)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(tempYAML)
			if writeErr != nil {
				setLastError(writeErr.Error())
			} else {
				setLastError(closeErr.Error())
			}
			return -2
		}
		defer func() { _ = os.Remove(tempYAML) }()
		usedYAMLConfig.Store(true)
		args = []string{
			"cloudflared",
			"tunnel",
			"--no-autoupdate",
			"--no-prechecks",
			"--loglevel", "fatal",
			"--transport-loglevel", "fatal",
			"--metrics", metricsAddr,
			"--config", tempYAML,
			"run",
		}
	default:
		args = []string{
			"cloudflared",
			"tunnel",
			"--no-autoupdate",
			"--no-prechecks",
			"--loglevel", "fatal",
			"--transport-loglevel", "fatal",
			"--metrics", metricsAddr,
		}
		if emptyConfigReset != "" {
			args = append(args, "--config", emptyConfigReset)
		}
		args = append(args, "run", "--token", parsed.token)
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	run := &tunnelRun{
		cancel:    cancelFn,
		shutdownC: make(chan struct{}),
		done:      make(chan struct{}),
	}

	stateMu.Lock()
	lastErr = ""
	activeTunnelURL = ""
	activeRun = run
	stateMu.Unlock()

	defer func() {
		run.stop()
		stateMu.Lock()
		if activeRun == run {
			activeRun = nil
		}
		activeTunnelURL = ""
		stateMu.Unlock()
		close(run.done)
		runtime.GC()
		debug.FreeOSMemory()
	}()

	bInfo := cliutil.GetBuildInfo("", "DEV")
	metrics.RegisterBuildInfo("", "", "DEV")
	tunnel.Init(bInfo, run.shutdownC)
	updater.Init(bInfo)

	if parsed.mode == "quick" {
		pollCtx, pollCancel := context.WithCancel(ctx)
		defer pollCancel()
		go pollQuickTunnelURL(pollCtx, metricsAddr)
	}

	app := &cli.App{
		Name:           "cloudflared",
		Flags:          tunnel.Flags(),
		Commands:       tunnel.Commands(),
		Action:         cliutil.ConfiguredAction(tunnel.TunnelCommand),
		Writer:         io.Discard,
		ErrWriter:      io.Discard,
		ExitErrHandler: func(_ *cli.Context, _ error) {},
	}

	runErr := app.RunContext(ctx, args)
	if runErr != nil && !run.stopped.Load() {
		setLastError(runErr.Error())
		return -3
	}
	return 0
}

//export StopCloudflared
func StopCloudflared(_ *C.char) (rc C.int) {
	defer func() {
		if r := recover(); r != nil {
			setLastError(fmt.Sprintf("panic: %v", r))
			rc = -4
		}
	}()

	stateMu.Lock()
	run := activeRun
	stateMu.Unlock()

	if run != nil {
		run.stop()
		select {
		case <-run.done:
		case <-time.After(3 * time.Second):
		}
	}
	return 0
}

//export CloudflaredIsRunning
func CloudflaredIsRunning() C.int {
	stateMu.Lock()
	running := activeRun != nil && !activeRun.stopped.Load()
	stateMu.Unlock()
	if running {
		return 1
	}
	return 0
}

//export CloudflaredLastError
func CloudflaredLastError() *C.char {
	stateMu.Lock()
	msg := lastErr
	stateMu.Unlock()
	if msg == "" {
		return nil
	}
	return C.CString(msg)
}

//export CloudflaredTunnelUrl
func CloudflaredTunnelUrl() *C.char {
	stateMu.Lock()
	u := activeTunnelURL
	stateMu.Unlock()
	if u == "" {
		return nil
	}
	return C.CString(u)
}

//export CloudflaredVersion
func CloudflaredVersion() *C.char {
	ver := "DEV"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/cloudflare/cloudflared" {
				ver = dep.Version
				break
			}
		}
	}
	return C.CString(ver)
}

//export CloudflaredFree
func CloudflaredFree(ptr *C.char) {
	if ptr != nil {
		C.free(unsafe.Pointer(ptr))
	}
}

func main() {}
