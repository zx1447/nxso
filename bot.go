/*
 * bot.go - Cloudflared 隧道 .so 封装 (无进程版)
 *
 * 直接导入 cloudflared 的 Go 代码作为库，在 goroutine 里运行，
 * 不创建任何子进程。cloudflared 运行在 JVM 进程内部。
 *
 * 编译:
 * go mod init bot
 * go get github.com/cloudflare/cloudflared/cmd/cloudflared/tunnel
 * CGO_ENABLED=1 go build -buildmode=c-shared -o bot.so bot.go
 */
package main

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cloudflare/cloudflared/cmd/cloudflared/tunnel"
)

var (
	mu         sync.Mutex
	shouldStop bool
	cancelFn   context.CancelFunc
	logFile    *os.File
)

func init() {
	// 初始化日志 - 使用 cache/.vip/.cf（PaperBootstrap 期望的位置）
	logDir := "cache/.vip/.cf"
	os.MkdirAll(logDir, 0755)
	var err error
	logFile, err = os.OpenFile(logDir+"/cf.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logFile = nil
	}
}

func logToFile(format string, args ...interface{}) {
	if logFile != nil {
		fmt.Fprintf(logFile, format, args...)
	}
}

//export StartCloudflared
func StartCloudflared(payload *C.char) C.int {
	goPayload := C.GoString(payload)
	if goPayload == "" {
		logToFile("[bot.so] empty payload\n")
		return -1
	}

	mu.Lock()
	if cancelFn != nil {
		mu.Unlock()
		logToFile("[bot.so] already running\n")
		return 0
	}
	shouldStop = false

	var tunnelURL string
	for _, line := range strings.Split(goPayload, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "url:") {
			tunnelURL = strings.TrimSpace(strings.TrimPrefix(line, "url:"))
			break
		}
	}
	if tunnelURL == "" && strings.HasPrefix(goPayload, "url:") {
		tunnelURL = strings.TrimSpace(strings.TrimPrefix(goPayload, "url:"))
	}

	if tunnelURL == "" {
		logToFile("[bot.so] no URL found in payload\n")
		mu.Unlock()
		return -1
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelFn = cancel
	mu.Unlock()

	go func() {
		defer func() {
			mu.Lock()
			cancelFn = nil
			mu.Unlock()
		}()

		logToFile("[bot.so] starting cloudflared tunnel to %s (no process, goroutine mode)\n", tunnelURL)

		err := tunnel.Run(ctx, []string{"cloudflared", "tunnel", "--url", tunnelURL, "--no-autoupdate", "--logfile", "cache/.vip/.cf/cf.log"})
		if err != nil && !shouldStop {
			logToFile("[bot.so] cloudflared error: %v\n", err)
		}
	}()

	logToFile("[bot.so] cloudflared started (no process, goroutine mode)\n")
	return 0
}

//export StopCloudflared
func StopCloudflared(_ *C.char) C.int {
	mu.Lock()
	shouldStop = true
	if cancelFn != nil {
		cancelFn()
		cancelFn = nil
	}
	mu.Unlock()

	time.Sleep(2 * time.Second)
	return 0
}

func main() {}
