# Cloudflared .so (Go 自编译版)

bot.so (45MB) - 内嵌 cloudflared，完全自包含，不需要额外下载二进制。

## Java 调用
```java
NativeLibrary lib = NativeLibrary.getInstance("/path/to/bot.so");
lib.getFunction("StartCloudflared").invokeInt(new Object[]{"your-token"});
lib.getFunction("StopCloudflared").invokeInt(new Object[]{""});
```
