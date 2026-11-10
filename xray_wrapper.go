package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"unsafe"

	applog "github.com/xtls/xray-core/app/log"
	commonlog "github.com/xtls/xray-core/common/log"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	_ "github.com/xtls/xray-core/main/distro/all"
)

type nopXrayLogHandler struct{}

func (nopXrayLogHandler) Handle(commonlog.Message) {}

func (nopXrayLogHandler) Severity() commonlog.Severity {
	return commonlog.Severity_Unknown
}

func init() {
	// Replace Xray-core's default StdoutLogWriter installed at package init()
	// so dlopen and LoadConfig() warnings never leak to the host stdout.
	commonlog.RegisterHandler(nopXrayLogHandler{})
}

var (
	xrayMutex     sync.Mutex
	xrayInstance  *core.Instance
	xrayLastError string
)

func suppressConsoleLogs(pbConfig *core.Config) {
	if pbConfig == nil {
		return
	}
	logConfigType := serial.GetMessageType(&applog.Config{})
	for i, appMsg := range pbConfig.App {
		if appMsg == nil || appMsg.Type != logConfigType {
			continue
		}
		msg, err := appMsg.GetInstance()
		if err != nil {
			continue
		}
		if logCfg, ok := msg.(*applog.Config); ok {
			changed := false
			if logCfg.ErrorLogType == applog.LogType_Console {
				logCfg.ErrorLogType = applog.LogType_None
				changed = true
			}
			if logCfg.AccessLogType == applog.LogType_Console {
				logCfg.AccessLogType = applog.LogType_None
				changed = true
			}
			if changed {
				pbConfig.App[i] = serial.ToTypedMessage(logCfg)
			}
		}
	}
}

func stopXrayLocked() {
	if xrayInstance != nil {
		func() {
			defer func() { recover() }()
			xrayInstance.Close()
		}()
		xrayInstance = nil
		commonlog.RegisterHandler(nopXrayLogHandler{})
		runtime.GC()
		debug.FreeOSMemory()
	}
}

//export StartXray
// StartXray 用 JSON 配置启动一个 Xray 实例（寄生在调用进程内, 不产生子进程）
// configJson: 完整的 Xray JSON 配置字符串 (wrapper 会自动抑制 Console stdout/stderr 日志)
// 返回: 0=成功, -1=配置解析失败, -2=创建实例失败, -3=启动失败, -4=内部 panic
// 重复调用时先校验配置解析，再停掉上一个实例并启动新实例
func StartXray(configJson *C.char) (rc C.int) {
	xrayMutex.Lock()
	defer xrayMutex.Unlock()
	xrayLastError = ""
	defer func() {
		if r := recover(); r != nil {
			commonlog.RegisterHandler(nopXrayLogHandler{})
			xrayLastError = fmt.Sprintf("panic: %v", r)
			rc = -4
		}
	}()

	if configJson == nil {
		xrayLastError = "null config pointer"
		return -1
	}

	cfg := C.GoString(configJson)
	if strings.TrimSpace(cfg) == "" {
		xrayLastError = "empty config"
		return -1
	}

	pbConfig, err := core.LoadConfig("json", strings.NewReader(cfg))
	if err != nil {
		xrayLastError = "parse config: " + err.Error()
		return -1
	}
	suppressConsoleLogs(pbConfig)

	stopXrayLocked()

	instance, err := core.New(pbConfig)
	if err != nil {
		commonlog.RegisterHandler(nopXrayLogHandler{})
		xrayLastError = "create instance: " + err.Error()
		return -2
	}
	if err := instance.Start(); err != nil {
		func() {
			defer func() { recover() }()
			instance.Close()
		}()
		commonlog.RegisterHandler(nopXrayLogHandler{})
		runtime.GC()
		debug.FreeOSMemory()
		xrayLastError = "start instance: " + err.Error()
		return -3
	}
	xrayInstance = instance
	return 0
}

//export StopXray
// StopXray 停止当前 Xray 实例。返回 0=已停止(或本来没在运行)
func StopXray() C.int {
	xrayMutex.Lock()
	defer xrayMutex.Unlock()
	defer func() { recover() }()
	stopXrayLocked()
	return 0
}

//export XrayIsRunning
// XrayIsRunning 返回当前 Xray 实例是否在运行 (1=运行中, 0=未运行)
func XrayIsRunning() C.int {
	xrayMutex.Lock()
	defer xrayMutex.Unlock()
	if xrayInstance != nil && xrayInstance.IsRunning() {
		return 1
	}
	return 0
}

//export XrayLastError
// XrayLastError 返回最近一次 StartXray 失败的原因字符串 (调用方负责调用 XrayFree / free 释放)
func XrayLastError() *C.char {
	xrayMutex.Lock()
	defer xrayMutex.Unlock()
	return C.CString(xrayLastError)
}

//export XrayVersion
// XrayVersion 返回编译进 .so 的 Xray-core 版本号 (调用方负责调用 XrayFree / free 释放)
func XrayVersion() *C.char {
	return C.CString(core.Version())
}

//export XrayFree
// XrayFree 释放由 XrayLastError / XrayVersion 分配的 C 字符串内存
func XrayFree(ptr unsafe.Pointer) {
	if ptr != nil {
		C.free(ptr)
	}
}

func main() {
	// Required for c-shared build mode
}
