package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"unsafe"

	box "github.com/sagernet/sing-box"
	boxconstant "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	boxlog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"
)

func init() {
	// Replace sing-box's package-level std logger so stray calls never write
	// to os.Stderr or trigger os.Exit(1) inside the host process.
	boxlog.SetStdLogger(boxlog.NewNOPFactory().Logger())
}

var (
	sbMutex     sync.Mutex
	sbInstance  *box.Box
	sbCancel    context.CancelFunc
	sbLastError string
)

// sbBaseCtx 带全部协议注册表的基础只读模板 context；
// 每次启动必须通过 service.ExtendContext(sbBaseCtx) 克隆独立注册表，避免实例间状态污染。
var sbBaseCtx = include.Context(context.Background())

func stopSingboxLocked() {
	stopped := false
	if sbCancel != nil {
		func() {
			defer func() { recover() }()
			sbCancel()
		}()
		sbCancel = nil
		stopped = true
	}
	if sbInstance != nil {
		func() {
			defer func() { recover() }()
			sbInstance.Close()
		}()
		sbInstance = nil
		stopped = true
	}
	if stopped {
		runtime.GC()
		debug.FreeOSMemory()
	}
}

//export StartSingbox
// StartSingbox 用 JSON 配置启动一个 sing-box 实例（寄生在调用进程内, 不产生子进程）
// configJson: 完整的 sing-box JSON 配置字符串 (wrapper 会强制禁用日志以防写 stderr 或 Fatal 调用 os.Exit)
// 返回: 0=成功, -1=配置解析失败, -2=创建实例失败, -3=启动失败, -4=内部 panic
// 重复调用时先校验配置解析，再停掉上一个实例并启动新实例
func StartSingbox(configJson *C.char) (rc C.int) {
	sbMutex.Lock()
	defer sbMutex.Unlock()
	sbLastError = ""
	defer func() {
		if r := recover(); r != nil {
			sbLastError = fmt.Sprintf("panic: %v", r)
			rc = -4
		}
	}()

	if configJson == nil {
		sbLastError = "null config pointer"
		return -1
	}

	cfg := C.GoString(configJson)
	if strings.TrimSpace(cfg) == "" {
		sbLastError = "empty config"
		return -1
	}

	ctx, cancel := context.WithCancel(service.ExtendContext(sbBaseCtx))
	options, err := singjson.UnmarshalExtendedContext[option.Options](ctx, []byte(cfg))
	if err != nil {
		cancel()
		sbLastError = "parse config: " + err.Error()
		return -1
	}

	// Force-disable logging in-process: sing-box's observableLogger writes to
	// os.Stderr when output=="" and calls os.Exit(1) on LevelFatal.
	options.Log = &option.LogOptions{Disabled: true}

	stopSingboxLocked()

	instance, err := box.New(box.Options{
		Context: ctx,
		Options: options,
	})
	if err != nil {
		cancel()
		sbLastError = "create service: " + err.Error()
		return -2
	}
	if err := instance.Start(); err != nil {
		cancel()
		func() {
			defer func() { recover() }()
			instance.Close()
		}()
		runtime.GC()
		debug.FreeOSMemory()
		sbLastError = "start service: " + err.Error()
		return -3
	}
	sbInstance = instance
	sbCancel = cancel
	return 0
}

//export StopSingbox
// StopSingbox 停止当前 sing-box 实例。返回 0=已停止(或本来没在运行)
func StopSingbox() C.int {
	sbMutex.Lock()
	defer sbMutex.Unlock()
	defer func() { recover() }()
	stopSingboxLocked()
	return 0
}

//export SingboxIsRunning
// SingboxIsRunning 返回当前 sing-box 实例是否在运行 (1=运行中, 0=未运行)
func SingboxIsRunning() C.int {
	sbMutex.Lock()
	defer sbMutex.Unlock()
	if sbInstance != nil {
		return 1
	}
	return 0
}

//export SingboxLastError
// SingboxLastError 返回最近一次 StartSingbox 失败的原因字符串 (调用方负责调用 SingboxFree / free 释放)
func SingboxLastError() *C.char {
	sbMutex.Lock()
	defer sbMutex.Unlock()
	return C.CString(sbLastError)
}

//export SingboxVersion
// SingboxVersion 返回编译进 .so 的 sing-box 版本号 (调用方负责调用 SingboxFree / free 释放)
func SingboxVersion() *C.char {
	return C.CString(boxconstant.Version)
}

//export SingboxFree
// SingboxFree 释放由 SingboxLastError / SingboxVersion 分配的 C 字符串内存
func SingboxFree(ptr unsafe.Pointer) {
	if ptr != nil {
		C.free(ptr)
	}
}

func main() {
	// Required for c-shared build mode
}
