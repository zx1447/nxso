# Cloudflared .so (Go 版, 进程内运行)

`bot.so` 现在把 **cloudflared 的 Go 代码直接作为依赖编译进 .so**，
通过 cloudflared 自己的 `tunnel run` 命令，在 **加载本 .so 的 JVM 进程内以 goroutine 运行隧道**。

## 与旧版的区别

| | 旧版 | 新版 |
|---|---|---|
| cloudflared 形态 | 二进制 `go:embed` 进 .so | 作为 Go 依赖**编译进** .so |
| 运行方式 | `exec.Command` 起**独立子进程** | 进程中 **goroutine** 运行 |
| 进程伪装 | 把子进程改名成 `java` | **不需要**（没有独立进程） |
| /dev/shm 释放 | 释放二进制 + 日志 | 不再释放二进制 |
| 停止方式 | 发信号给子进程 | 取消内部 `context` |

因为隧道运行在 JVM 进程内，**`ps` 里看不到任何 cloudflared 进程**，也就无需再伪装成 java。

## Java 调用（签名不变，无需改动）

```java
NativeLibrary lib = NativeLibrary.getInstance("/path/to/bot.so");
// payload 是 tunnel token (>50 字符)，或完整 cloudflared 配置 yaml
lib.getFunction("StartCloudflared").invokeInt(new Object[]{"your-token"});
lib.getFunction("StopCloudflared").invokeInt(new Object[]{""});
```

- `StartCloudflared(payload)`：
  - 若 `payload` 看起来像 token（长度 >50 且不以 `url:`/`http` 开头）→ `tunnel run --token <payload>`
  - 否则视为 cloudflared 配置 yaml → 写临时文件后 `tunnel run --config <file>`
- `StopCloudflared("")`：取消内部 context，进程内隧道优雅退出。

## 构建

由 `.github/workflows/build.yml` 的 `build-bot-so` job 在 ubuntu 上完成：

```bash
# 需要 Go 1.26 (cloudflared 依赖) + gcc (cgo)
go mod tidy                       # 解析依赖并生成 go.sum
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
  go build -buildmode=c-shared -o bot.so bot.go
```

产物 `bot.so`（+ `bot.h`）会被自动提交回仓库。

## 依赖说明

`go.mod` 直接 `require github.com/cloudflare/cloudflared v2026.7.3`，
并**原样保留 cloudflared 自己的 replace 指令**（urfave/cli fork、quic-go fork 等）。
这些 replace 不会跨模块自动传递，作为下游导入者必须在本模块里也声明，否则会退回官方包、
与 cloudflared 代码中使用的 fork API 不匹配而**编译失败**。

## 注意事项

- 隧道日志输出到 stderr（被 JVM / 面板捕获）。旧版写到 `/dev/shm/.cf/cf.log` 的行为已移除。
- `cloudflared` 内部会调用 `os.Exit`；本 .so 已把 `cli.App` 的 `OsExiter` /
  `ExitErrHandler` 覆盖为空函数，确保隧道出错时**不会把整个 JVM 进程干掉**。
- 同进程内 `Stop` 后再 `Start` 是安全的（cloudflared 的指标注册用 `sync.Once` 保护，不会重复注册 panic）。
