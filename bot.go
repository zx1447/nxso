package main

import "C"
import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/urfave/cli/v2"
	"go.uber.org/automaxprocs/maxprocs"

	"github.com/cloudflare/cloudflared/cmd/cloudflared/cliutil"
	"github.com/cloudflare/cloudflared/cmd/cloudflared/tunnel"
	"github.com/cloudflare/cloudflared/cmd/cloudflared/updater"
	"github.com/cloudflare/cloudflared/metrics"
)

var (
	mu        sync.Mutex
	cancel    context.CancelFunc
	shutdownC chan struct{}
)

//export StartCloudflared
func StartCloudflared(payload *C.char) C.int {
	payloadStr := C.GoString(payload)

	// Required by cloudflared for QUIC
	os.Setenv("QUIC_GO_DISABLE_ECN", "1")
	_, _ = maxprocs.Set()

	mu.Lock()
	ctx, cancelFn := context.WithCancel(context.Background())
	cancel = cancelFn
	shutdownC = make(chan struct{})
	mu.Unlock()

	// Initialize cloudflared subsystems
	bInfo := cliutil.GetBuildInfo("", "DEV")
	metrics.RegisterBuildInfo("", "", "DEV")
	tunnel.Init(bInfo, shutdownC)
	updater.Init(bInfo)

	// Set up log file path
	logDir := "cache/.vip/.cf"
	os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, "cf.log")

	// Build CLI args based on payload type
	args := buildArgs(payloadStr, logFile)

	// Create CLI app
	app := &cli.App{
		Name:     "cloudflared",
		Flags:    tunnel.Flags(),
		Commands: tunnel.Commands(),
		Action:   cliutil.ConfiguredAction(tunnel.TunnelCommand),
	}

	// Run the app (blocking)
	err := app.RunContext(ctx, args)
	if err != nil {
		return 1
	}
	return 0
}

//export StopCloudflared
func StopCloudflared(_ *C.char) C.int {
	mu.Lock()
	defer mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if shutdownC != nil {
		close(shutdownC)
	}
	return 0
}

func buildArgs(payload string, logFile string) []string {
	// Check if payload is config-style (quick tunnel) or token-based
	if strings.Contains(payload, "\n") || strings.HasPrefix(payload, "url:") {
		// Quick tunnel - parse config-style payload
		// Format: "url: http://127.0.0.1:PORT\nno-autoupdate: true\nprotocol: http2\n"
		url := ""
		protocol := ""
		for _, line := range strings.Split(payload, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "url:") {
				url = strings.TrimSpace(strings.TrimPrefix(line, "url:"))
			} else if strings.HasPrefix(line, "protocol:") {
				protocol = strings.TrimSpace(strings.TrimPrefix(line, "protocol:"))
			}
		}

		args := []string{
			"cloudflared",
			"--no-autoupdate",
			"--logfile", logFile,
		}
		if url != "" {
			args = append(args, "--url", url)
		}
		if protocol != "" {
			args = append(args, "--protocol", protocol)
		}
		return args
	}

	// Token-based tunnel
	return []string{
		"cloudflared",
		"tunnel", "run",
		"--no-autoupdate",
		"--no-prechecks",
		"--logfile", logFile,
		"--token", payload,
	}
}

func main() {}
