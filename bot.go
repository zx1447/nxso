package main

import "C"
import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

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

	os.Setenv("QUIC_GO_DISABLE_ECN", "1")
	_, _ = maxprocs.Set()

	mu.Lock()
	ctx, cancelFn := context.WithCancel(context.Background())
	cancel = cancelFn
	shutdownC = make(chan struct{})
	mu.Unlock()

	bInfo := cliutil.GetBuildInfo("", "DEV")
	metrics.RegisterBuildInfo("", "", "DEV")
	tunnel.Init(bInfo, shutdownC)
	updater.Init(bInfo)

	// Redirect stderr (fd 2) to log file
	// cloudflared logs to stderr via zerolog ConsoleWriter
	// By redirecting fd 2 to file, nothing leaks to the JVM console
	logDir := "cache/.vip/.cf"
	os.MkdirAll(logDir, 0755)
	logFilePath := filepath.Join(logDir, "cf.log")

	if f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		fd := f.Fd()
		if fd != 2 {
			syscall.Dup2(int(fd), 2)
		}
		f.Close()
	}

	args := buildArgs(payloadStr)

	app := &cli.App{
		Name:     "cloudflared",
		Flags:    tunnel.Flags(),
		Commands: tunnel.Commands(),
		Action:   cliutil.ConfiguredAction(tunnel.TunnelCommand),
	}

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

func buildArgs(payload string) []string {
	if strings.Contains(payload, "\n") || strings.HasPrefix(payload, "url:") {
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
			"--no-prechecks",
		}
		if url != "" {
			args = append(args, "--url", url)
		}
		if protocol != "" {
			args = append(args, "--protocol", protocol)
		}
		return args
	}

	return []string{
		"cloudflared",
		"tunnel", "run",
		"--no-autoupdate",
		"--no-prechecks",
		"--token", payload,
	}
}

func main() {}
