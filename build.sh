#!/bin/bash
# build.sh - 编译 bot.so (Cloudflared 隧道 .so 封装)
#
# 用法:
#   ./build.sh              # 编译 amd64 版本
#   ./build.sh arm64        # 编译 arm64 版本
#   ./build.sh c            # 编译 C 版本（简单版，不嵌入二进制）

set -e

ARCH="${1:-amd64}"
VERSION="${2:-c}"  # 默认用 C 版本

echo "============================================"
echo "  Cloudflared .so 编译器"
echo "  架构: $ARCH"
echo "============================================"

if [ "$VERSION" = "c" ]; then
    echo ""
    echo "=== 编译 C 版本 ==="
    echo ""
    
    gcc -shared -fPIC -o bot.so bot.c -lpthread -O2
    echo "✅ bot.so 编译完成"
    ls -lh bot.so
    echo ""
    echo "用法: 把 bot.so 放到服务器上，确保 cloudflared 二进制在以下路径之一:"
    echo "  - /dev/shm/.cf_real"
    echo "  - /tmp/.cf_real"
    echo "  - 系统 PATH 里的 cloudflared"
    echo "  - 环境变量 CF_BIN 指定"
    
elif [ "$VERSION" = "go" ]; then
    echo ""
    echo "=== 编译 Go 版本（嵌入 cloudflared）==="
    echo ""
    
    # 检查 Go
    if ! command -v go &>/dev/null; then
        echo "❌ 需要安装 Go: https://go.dev/dl/"
        exit 1
    fi
    
    # 下载 cloudflared 二进制
    if [ "$ARCH" = "arm64" ]; then
        CF_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64"
    else
        CF_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64"
    fi
    
    echo "下载 cloudflared ($ARCH)..."
    curl -fsSL -o cloudflared "$CF_URL"
    chmod +x cloudflared
    ls -lh cloudflared
    
    # 初始化 Go 模块
    if [ ! -f go.mod ]; then
        go mod init bot
    fi
    
    # 编译
    echo "编译 bot.so..."
    CGO_ENABLED=1 GOARCH=$ARCH go build -buildmode=c-shared -o bot.so bot.go
    
    echo ""
    echo "✅ bot.so 编译完成（含嵌入的 cloudflared）"
    ls -lh bot.so
    
    # 清理
    rm -f cloudflared
fi

echo ""
echo "============================================"
echo "  编译完成!"
echo "============================================"
echo ""
echo "文件列表:"
ls -lh bot.so bot.h 2>/dev/null || ls -lh bot.so
echo ""
echo "Java JNA 调用示例:"
echo '  NativeLibrary lib = NativeLibrary.getInstance("/path/to/bot.so");'
echo '  Function start = lib.getFunction("StartCloudflared");'
echo '  start.invokeInt(new Object[]{"your-cf-token-here"});'
