# nxso — xs 分支（Xray / sing-box 的 .so 构建）

把 Xray-core 和 sing-box 编译成 c-shared 动态库（`.so`），供宿主进程直接 dlopen/FFI 加载，不落盘、不产生子进程。

## 产物清单

| 文件 | 内容 |
|---|---|
| `xray.so` / `xray.h` | Xray-core **v26.11.9**，linux/amd64 |
| `xray_arm64.so` | Xray-core v26.11.9，linux/arm64 |
| `singbox.so` / `singbox.h` | sing-box **v1.14.4**，linux/amd64，tags：`with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,badlinkname,tfogo_checklinkname0` |
| `singbox_arm64.so` | sing-box v1.14.4，linux/arm64（同 tags） |
| `xray_wrapper.go` | Xray 包装源码（CI 拷入上游 `libxray/main.go` 编译） |
| `singbox_wrapper.go` | sing-box 包装源码（CI 拷入上游 `cmd/singboxso/main.go` 编译） |

> 注：未开启 `with_naive_outbound`（其 cronet 静态库与部分环境 glibc 不兼容，且上游 `DEFAULT_BUILD_TAGS_OTHERS` 默认也不含它）。如需 `tailscale` / `openvpn` / `openconnect` / `cloudflared` 等协议，可在 `build-xs.yml` 的 `TAGS` 中补充对应 tag。

## 导出 API（C ABI）

两个库签名完全对称，`configJson` 均为完整 JSON 配置字符串：

| 符号 | C 签名 | 说明 |
|---|---|---|
| `StartXray` / `StartSingbox` | `int Start(const char* configJson)` | 返回 `0`=成功，`-1`=配置解析失败，`-2`=创建实例失败，`-3`=启动失败，`-4`=内部 panic；**重复调用会先校验新配置，再停掉旧实例并启动新实例** |
| `StopXray` / `StopSingbox` | `int Stop(void)` | 停止当前实例并返回 `0` |
| `XrayIsRunning` / `SingboxIsRunning` | `int IsRunning(void)` | 返回 `1`=运行中，`0`=未运行 |
| `XrayLastError` / `SingboxLastError` | `char* LastError(void)` | 最近一次失败原因；需调用 `XrayFree` / `SingboxFree`（或 C `free()`）释放 |
| `XrayVersion` / `SingboxVersion` | `char* Version(void)` | 编译进 .so 的版本号；需调用 `XrayFree` / `SingboxFree`（或 C `free()`）释放 |
| `XrayFree` / `SingboxFree` | `void Free(void* ptr)` | 释放 `LastError` / `Version` 返回的 C 字符串内存 |

## 配置与兼容注意（重要）

1. **宿主日志与进程保护**：
   - `xray.so` 已替换默认 `StdoutLogWriter` 并自动把 `Console` 日志类型降为 `None`（仍建议使用 `"log": {"loglevel": "none", "access": "none", "error": "none"}`）。
   - `singbox.so` 已屏蔽全局 `StdLogger` 并强制覆盖 `options.Log = { disabled: true }`，避免日志输出到宿主 `stderr` 或触发 `LevelFatal` 调用 `os.Exit(1)` 杀掉宿主进程。
2. **sing-box 1.13+ WireGuard 语法变化**：
   - sing-box v1.14.4 已**彻底移除**旧版 `"type": "wireguard"` outbound；使用 WARP/WireGuard 时必须改为 `endpoints: [{ "type": "wireguard", ... }]` 新语法。
3. **Xray geoip/geosite 资源路径**：
   - 在宿主进程（如 `node`、`java`）内加载 `.so` 时，Xray 默认按 `os.Executable()`（即 `/usr/bin/node`）所在目录寻找 `geoip.dat` / `geosite.dat`。若配置用到了 `geoip:` / `geosite:` 规则，请设置环境变量 `XRAY_LOCATION_ASSET`，或在配置 JSON 中加上 `"env": { "xray.location.asset": "/path/to/assets" }`。

## Node.js (koffi) 调用示例

> ⚠️ 不要把返回指针声明为 `'const char*'` 后再调 `koffi.decode(ptr, 'str')`：koffi 会先自动转成 JS 字符串导致 `koffi.decode` 报错，且原始指针丢失无法释放内存。请声明为 `'void*'` 再解码并调用 `*Free()`，且对耗时启动推荐使用 `.async()` 避免阻塞事件循环：

```js
const koffi = require('koffi');
const lib = koffi.load('/path/to/singbox.so');

const StartSingbox     = lib.func('int StartSingbox(const char*)');
const StopSingbox      = lib.func('int StopSingbox()');
const SingboxIsRunning = lib.func('int SingboxIsRunning()');
const LastErrorPtr     = lib.func('void* SingboxLastError()');
const VersionPtr       = lib.func('void* SingboxVersion()');
const FreeStr          = lib.func('void SingboxFree(void*)');

function takeCString(ptr) {
    if (!ptr) return '';
    try {
        return koffi.decode(ptr, 'str');
    } finally {
        FreeStr(ptr);
    }
}

console.log('version:', takeCString(VersionPtr()));

// 推荐异步调用避免阻塞 Node.js 事件循环（如 ACME / WireGuard / 远程规则下载）
StartSingbox.async(JSON.stringify(config), (err, rc) => {
    if (err) return console.error('ffi error:', err);
    if (rc !== 0) {
        console.error('start failed:', rc, takeCString(LastErrorPtr()));
        return;
    }
    console.log('running:', SingboxIsRunning() === 1);
});
```

## 重新构建 / 升级版本

- push 到 `xs` 分支或手动 `workflow_dispatch` 触发 `.github/workflows/build-xs.yml`；
- 产物（4 个 `.so` + 2 个 `.h`）由 Actions 自动编译并回写到本分支；
- 升级上游版本：修改 `build-xs.yml` 中对应 `--branch`、版本号及核对 upstream `DEFAULT_BUILD_TAGS_OTHERS` / `LDFLAGS`。
