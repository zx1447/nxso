# Cloudflared .so (Go 版, 进程内运行)

`bot.so` / `bot_arm64.so` 把 **cloudflared Go 核心直接编译为 c-shared 动态库**，可在 Node.js (`koffi`) / JVM 等宿主进程内直接加载，实现 **0 子进程运行**、安全重复启停与 Quick Tunnel 域名内存获取。

## 导出 C 接口

| 函数签名 | 说明 |
|---|---|
| `int StartCloudflared(const char* payload)` | 阻塞运行隧道（JS 中必须用 `koffi` `.async()` 调用）。自动安全停止旧实例并等待退出后重启。支持 Quick Tunnel (`url: ...`/`protocol: ...`)、Fixed Tunnel Token 与 YAML 配置。返回 `0` 正常退出/停止，`-1` 参数错误，`-2` 构建失败，`-3` 运行错误，`-4` panic 捕获 |
| `int StopCloudflared(const char* _)` | 幂等停止当前隧道并等待退出（可多次调用，不 panic） |
| `int CloudflaredIsRunning(void)` | `1` 运行中，`0` 未运行 |
| `char* CloudflaredTunnelUrl(void)` | 返回 Quick Tunnel 域名（如 `xxx.trycloudflare.com`）；无则返回 `NULL`，非空需调用 `CloudflaredFree` |
| `char* CloudflaredLastError(void)` | 返回最近一次错误信息；无则返回 `NULL`，非空需调用 `CloudflaredFree` |
| `char* CloudflaredVersion(void)` | 返回 cloudflared 模块版本；非空需调用 `CloudflaredFree` |
| `void CloudflaredFree(char* ptr)` | 释放以上返回的字符串指针 |

## 修复说明

- **不劫持 `fd 2` / 不写入磁盘日志**：移除 `syscall.Dup2` 与 `cache/.vip/.cf/cf.log`，默认静默日志，通过本地回环 `/quicktunnel` 接口在内存中提取 Quick Tunnel 域名。
- **不杀死宿主进程**：覆盖 `cli.OsExiter`、`cli.ErrWriter` 与 `app.ExitErrHandler`，并在 `StartCloudflared`/`StopCloudflared` 入口加入 panic `recover()`。
- **可安全多次启停与切换模式**：每次启动隔离 `prometheus.DefaultRegisterer` 并清空全局 `config.Configuration`，使用 `sync.Once` 防止重复关闭 `shutdownC`。
- **支持双架构**：CI 自动构建 `bot.so` (`linux/amd64`) 与 `bot_arm64.so` (`linux/arm64`)。
