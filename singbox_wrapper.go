package main

/*
#include <stdlib.h>
*/
import "C"

import (
        "context"
        "strings"
        "sync"

        box "github.com/sagernet/sing-box"
        boxconstant "github.com/sagernet/sing-box/constant"
        "github.com/sagernet/sing-box/include"
        "github.com/sagernet/sing-box/option"
)

var (
        sbMutex      sync.Mutex
        sbInstance   *box.Box
        sbCancel     context.CancelFunc
        sbLastError  string
)

// sbBaseCtx 带全部协议注册表的基础 context (解析配置和创建实例必须用同一个,
// 否则 DNS transport / 出站类型会报 "unknown type")
var sbBaseCtx = include.Context(context.Background())

func stopSingboxLocked() {
        if sbInstance != nil {
                func() {
                        defer func() { recover() }()
                        sbInstance.Close()
                }()
                sbInstance = nil
        }
        if sbCancel != nil {
                func() {
                        defer func() { recover() }()
                        sbCancel()
                }()
                sbCancel = nil
        }
}

//export StartSingbox
// StartSingbox 用 JSON 配置启动一个 sing-box 实例（寄生在调用进程内, 不产生子进程）
// configJson: 完整的 sing-box JSON 配置字符串 (log 建议设 {"disabled":true},
//             否则日志会打到宿主进程的 stdout/stderr)
// 返回: 0=成功, -1=配置解析失败, -2=创建实例失败, -3=启动失败
// 重复调用会先自动停掉上一个实例再启动新的
func StartSingbox(configJson *C.char) C.int {
        sbMutex.Lock()
        defer sbMutex.Unlock()
        sbLastError = ""
        defer func() {
                if r := recover(); r != nil {
                        sbLastError = "panic: protect host process"
                }
        }()

        cfg := C.GoString(configJson)
        if strings.TrimSpace(cfg) == "" {
                sbLastError = "empty config"
                return -1
        }

        stopSingboxLocked()

        var options option.Options
        if err := options.UnmarshalJSONContext(sbBaseCtx, []byte(cfg)); err != nil {
                sbLastError = "parse config: " + err.Error()
                return -1
        }

        ctx, cancel := context.WithCancel(sbBaseCtx)
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
                func() {
                        defer func() { recover() }()
                        instance.Close()
                }()
                cancel()
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

//export SingboxLastError
// SingboxLastError 返回最近一次 StartSingbox 失败的原因字符串 (调用方负责 C.free / koffi.free)
func SingboxLastError() *C.char {
        sbMutex.Lock()
        defer sbMutex.Unlock()
        return C.CString(sbLastError)
}

//export SingboxVersion
// SingboxVersion 返回编译进 .so 的 sing-box 版本号 (调用方负责 C.free / koffi.free)
func SingboxVersion() *C.char {
        return C.CString(boxconstant.Version)
}

func main() {
        // Required for c-shared build mode
}
