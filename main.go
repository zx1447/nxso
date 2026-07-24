package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"os"
	"time"

	// 引入官方哪吒探针 C-Core 执行命令包
	"github.com/nezhahq/agent/cmd/agent"
)

//export Hello
func Hello() *C.char {
	return C.CString("Hello from nxso!")
}

//export Version
func Version() *C.char {
	return C.CString("2.0.0")
}

// StartNezha 供 Node.js 调用，在后台静默启动探针
//export StartNezha
func StartNezha(addr *C.char, key *C.char, tls C.int) {
	// 1. 将 C 字符串转换为 Go 字符串并写入环境变量
	os.Setenv("NZ_SERVER", C.GoString(addr))
	os.Setenv("NZ_CLIENT_SECRET", C.GoString(key))
	if int(tls) == 1 {
		os.Setenv("NZ_TLS", "true")
	} else {
		os.Setenv("NZ_TLS", "false")
	}

	// 2. 开启一个独立的 Go 程（Goroutine）在后台线程中运行探针，不阻塞 Node.js 主线程
	go func() {
		agent.Execute()
	}()

	// 3. 给探针初始化一点点启动缓冲时间
	time.Sleep(500 * time.Millisecond)
}

func main() {
	// CGO 编译共享库必须保留 main 函数
}
