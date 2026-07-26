/*
 * bot.go - Cloudflared 隧道 .so 封装 (Go 版, 进程内运行)
 *
 * 与旧版的区别：
 *   旧版把 cloudflared 二进制 go:embed 进 .so，运行时释放到 /dev/shm，
 *   再用 exec.Command 起一个【独立子进程】，并用各种手段把进程名伪装成 "java"。
 *
 *   新版直接把 cloudflared 的 Go 代码作为依赖编译进本 .so，
 *   通过 cloudflared 自己的 tunnel 命令在【goroutine 里、加载本 .so 的 JVM 进程内】
 *   启动隧道。没有独立进程、没有二进制释放、不需要改名伪装。
 *
 * 编译步骤 (在 CI / linux amd64 上)：
 *   CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -buildmode=c-shared -o bot.so bot.go
 * 产物: bot.so (+ bot.h)。Java 端用 JNA 调 StartCloudflared / StopCloudflared，
 * 函数签名与旧版完全一致，Java 侧无需改动。
 *
 * 注意：
 *   - 本文件 import 了 github.com/cloudflare/cloudflared/cmd/cloudflared/tunnel 等包，
 *     go.mod 里必须带上 cloudflared 的 replace 指令（尤其是 urfave/cli fork 和 quic-go fork），
 *     否则会用回官方包导致编译失败。
 *   - cloudflared 内部会调用 cli.Exit / os.Exit；在 JVM 进程内必须把 OsExiter /
 *     ExitErrHandler 覆盖成空函数，否则隧道一出错就会把整个 JVM 干掉。
 *   - 隧道停止靠取消我们传入的 context 实现，因此 StopCloudflared 直接 cancel 即可。
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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudflare/cloudflared/cmd/cloudflared/cliutil"
	"github.com/cloudflare/cloudflared/cmd/cloudflared/tunnel"
	"github.com/urfave/cli/v2"
)

// 进程内 cloudflared 状态
var (
	mu         sync.Mutex
	cfCancel   context.CancelFunc // 取消它即可优雅停止隧道
	shouldStop bool
	logDir     string
	initOnce   sync.Once // tunnel.Init 只需调用一次
)

func init() {
	// 日志 / 临时配置目录：优先 /dev/shm（RAM），回退 /tmp
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		logDir = "/dev/shm/.cf"
	} else {
		logDir = filepath.Join(os.TempDir(), ".cf")
	}
	_ = os.MkdirAll(logDir, 0755)
}

// isToken 判断 payload 是 token 还是完整 cloudflared 配置 yaml
func isToken(payload string) bool {
	if strings.HasPrefix(payload, "url:") || strings.HasPrefix(payload, "http") {
		return false
	}
	return len(payload) > 50
}

//export StartCloudflared
//
// 在 JVM 进程内启动 cloudflared 隧道 (goroutine)。
// payload: 要么是 tunnel token (>50 字符, 且不以 url: / http 开头),
//         要么是完整的 cloudflared 配置 yaml (含 tunnel: <id> 与 credentials 等)。
func StartCloudflared(payload *C.char) C.int {
	goPayload := C.GoString(payload)
	if goPayload == "" {
		fmt.Fprintf(os.Stderr, "[bot.so] empty payload\n")
		return -1
	}

	mu.Lock()
	if cfCancel != nil {
		mu.Unlock()
		fmt.Fprintf(os.Stderr, "[bot.so] cloudflared already running in-process\n")
		return 0
	}
	shouldStop = false
	mu.Unlock()

	go runCloudflaredInProcess(goPayload)
	return 0
}

//export StopCloudflared
//
// 取消 context，进程内的隧道 goroutine 会优雅退出。
func StopCloudflared(_ *C.char) C.int {
	mu.Lock()
	shouldStop = true
	if cfCancel != nil {
		cfCancel()
	}
	mu.Unlock()
	_ = os.Remove(filepath.Join(logDir, "cf.log"))
	return 0
}

// runCloudflaredInProcess 复用 cloudflared 自己的 tunnel 命令，在进程内跑隧道。
func runCloudflaredInProcess(payload string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[bot.so] cloudflared panic recovered: %v\n", r)
		}
		mu.Lock()
		cfCancel = nil
		mu.Unlock()
	}()

	// 这个 context 驱动整个隧道生命周期；Stop 时 cancel 它即可停止。
	ctx, cancel := context.WithCancel(context.Background())
	mu.Lock()
	cfCancel = cancel
	mu.Unlock()

	var args []string
	if isToken(payload) {
		args = []string{
			"cloudflared", "tunnel", "run",
			"--protocol", "auto",
			"--no-autoupdate",
			"--token", payload,
		}
	} else {
		// payload 是 cloudflared 配置 yaml，写到临时文件后用 --config 加载
		cfgPath := filepath.Join(logDir, fmt.Sprintf("cf_config_%d.yml", os.Getpid()))
		if err := os.WriteFile(cfgPath, []byte(payload), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "[bot.so] write config failed: %v\n", err)
			return
		}
		defer os.Remove(cfgPath)
		args = []string{
			"cloudflared", "tunnel", "run",
			"--no-autoupdate",
			"--config", cfgPath,
		}
	}

	// 初始化 cloudflared tunnel 包（设置内部 buildInfo / graceShutdownC 全局变量）。
	// 只需一次；用 sync.Once 保护。
	initOnce.Do(func() {
		tunnel.Init(cliutil.GetBuildInfo("", "DEV"), make(chan struct{}))
	})

	app := &cli.App{
		Name:    "cloudflared",
		Version: "in-process",
		Commands: tunnel.Commands(),
		// 关键：cloudflared 内部会用 cli.Exit / os.Exit，在 JVM 进程内必须拦截，
		// 否则隧道出错会直接把整个 JVM 干掉。
		OsExiter:      func(int) {},
		ExitErrHandler: func(*cli.Context, error) {},
	}

	fmt.Fprintf(os.Stderr, "[bot.so] starting cloudflared in-process (goroutine inside JVM)\n")
	if err := app.RunContext(ctx, args); err != nil {
		if !shouldStop {
			fmt.Fprintf(os.Stderr, "[bot.so] cloudflared exited with error: %v\n", err)
		}
	}
}

func main() {
	// c-shared 模式需要 main 函数但不会执行
	_ = time.Second
}
