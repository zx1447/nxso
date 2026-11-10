# nxso — xs 分支（Xray / sing-box 的 .so 构建）

把 Xray-core 和 sing-box 编译成 c-shared 动态库（`.so`），供遨游面板在宿主进程内直接 dlopen/FFI 加载，不落盘、不产生子进程。与 `main` 分支的 `nezha_agent.so` 同一套路。

## 产物清单

| 文件 | 内容 |
|---|---|
| `xray.so` / `xray.h` | Xray-core **v26.11.9**，linux/amd64 |
| `xray_arm64.so` | Xray-core v26.11.9，linux/arm64 |
| `singbox.so` / `singbox.h` | sing-box **v1.14.4**，linux/amd64，功能 tags：`with_gvisor,with_quic,with_grpc,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api` |
| `singbox_arm64.so` | sing-box v1.14.4，linux/arm64（同 tags） |
| `xray_wrapper.go` | Xray 包装源码（CI 拷入上游 `libxray/main.go` 编译） |
| `singbox_wrapper.go` | sing-box 包装源码（CI 拷入上游 `cmd/singboxso/main.go` 编译） |

> 注：v1.14.4 构建时移除了 `with_naive_outbound`（其 cronet 静态库与部分环境 glibc 不兼容）。

## 导出 API（C ABI）

两个库签名完全对称，`configJson` 均为完整 JSON 配置字符串：

| 符号 | C 签名 | 说明 |
|---|---|---|
| `StartXray` / `StartSingbox` | `int Start(const char* configJson)` | 返回 `0`=成功，`-1`=配置解析失败，`-2`=创建实例失败，`-3`=启动失败；**重复调用会先自动停掉旧实例再启动新实例** |
| `StopXray` / `StopSingbox` | `int Stop(void)` | 停止当前实例，`0`=已停止 |
| `XrayLastError` / `SingboxLastError` | `const char* LastError(void)` | 最近一次失败原因；返回的指针需调用方 `free()` |
| `XrayVersion` / `SingboxVersion` | `const char* Version(void)` | 编译进 .so 的版本号；返回的指针需调用方 `free()` |

注意：失败原因与版本字符串由 `C.CString` 分配，C 加载器记得 `free()`；koffi 用 `koffi.free(ptr)`。

## 配置注意（重要）

- Xray 配置建议 `"log": {"loglevel": "none", "access": "", "error": ""}`；
- sing-box 配置建议 `"log": {"disabled": true}`；
- 否则核心日志会打到宿主进程的 stdout/stderr，容易暴露。

## Node.js (koffi) 调用示例

```js
const koffi = require('koffi');
const lib = koffi.load('/path/to/singbox.so');
const StartSingbox = lib.func('int StartSingbox(const char*)');
const StopSingbox  = lib.func('int StopSingbox()');
const LastError    = lib.func('const char* SingboxLastError()');
const Version      = lib.func('const char* SingboxVersion()');

console.log('version:', koffi.decode(Version(), 'str'));
const rc = StartSingbox(JSON.stringify(config));
if (rc !== 0) console.log('err:', koffi.decode(LastError(), 'str'));
// 停止: StopSingbox()
```

C 加载器示例（编译: `gcc loader.c -ldl -o loader`）见 `main` 分支 nezha 同款思路：`dlopen` → `dlsym` → 传参调用。

## 重新构建 / 升级版本

- push 到 `xs` 分支或手动 `workflow_dispatch` 触发 `.github/workflows/build-xs.yml`；
- 产物（4 个 .so + 2 个 .h）由 Actions 自动编译并回写到本分支；
- 升级上游版本：改 `build-xs.yml` 里 `--branch` 和 ldflags 中两处 tag（sing-box 大版本升级需先核对 `singbox_wrapper.go` 的 API，1.14 与 1.12 的 `box.New` 签名不同）。

## 本分支已验证（本地 linux/amd64 实测）

- `dlopen` 加载 → `StartXray`/`StartSingbox` rc=0 → 本地 SOCKS 入站 curl 出网成功 → `Stop` rc=0；
- 版本号输出 `26.11.9` / `v1.14.4`。
