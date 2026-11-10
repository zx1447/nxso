package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"strings"
	"sync"

	"github.com/xtls/xray-core/core"
	_ "github.com/xtls/xray-core/main/distro/all"
)

var (
	xrayMutex     sync.Mutex
	xrayInstance  *core.Instance
	xrayLastError string
)

func stopXrayLocked() {
	if xrayInstance != nil {
		func() {
			defer func() { recover() }()
			xrayInstance.Close()
		}()
		xrayInstance = nil
	}
}

//export StartXray
// StartXray 用 JSON 配置启动一个 Xray 实例（寄生在调用进程内, 不产生子进程）
// configJson: 完整的 Xray JSON 配置字符串 (log 建议设 loglevel:"none", access/error 留空,
//             否则日志会打到宿主进程的 stdout/stderr)
// 返回: 0=成功, -1=配置解析失败, -2=创建实例失败, -3=启动失败
// 重复调用会先自动停掉上一个实例再启动新的
func StartXray(configJson *C.char) C.int {
	xrayMutex.Lock()
	defer xrayMutex.Unlock()
	xrayLastError = ""
	defer func() {
		if r := recover(); r != nil {
			xrayLastError = "panic: protect host process"
		}
	}()

	cfg := C.GoString(configJson)
	if strings.TrimSpace(cfg) == "" {
		xrayLastError = "empty config"
		return -1
	}

	stopXrayLocked()

	pbConfig, err := core.LoadConfig("json", strings.NewReader(cfg))
	if err != nil {
		xrayLastError = "parse config: " + err.Error()
		return -1
	}
	instance, err := core.New(pbConfig)
	if err != nil {
		xrayLastError = "create instance: " + err.Error()
		return -2
	}
	if err := instance.Start(); err != nil {
		func() {
			defer func() { recover() }()
			instance.Close()
		}()
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

//export XrayLastError
// XrayLastError 返回最近一次 StartXray 失败的原因字符串 (调用方负责 C.free / koffi.free)
func XrayLastError() *C.char {
	xrayMutex.Lock()
	defer xrayMutex.Unlock()
	return C.CString(xrayLastError)
}

//export XrayVersion
// XrayVersion 返回编译进 .so 的 Xray-core 版本号 (调用方负责 C.free / koffi.free)
func XrayVersion() *C.char {
	return C.CString(core.Version())
}

func main() {
	// Required for c-shared build mode
}
