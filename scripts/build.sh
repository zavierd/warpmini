#!/bin/bash

# WarpMini 自动化构建脚本
# 支持Intel芯片(AMD64)和M芯片(ARM64)

echo "🚀 开始构建 WarpMini..."

# 清理之前的构建文件
echo "🧹 清理旧文件..."
rm -f warpmini-macos-amd64 warpmini-macos-arm64
rm -f warpmini-macos-amd64.zip warpmini-macos-arm64.zip

# 构建Intel芯片版本 (AMD64)
echo "🔧 构建Intel芯片版本 (AMD64)..."
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o warpmini-macos-amd64 .
if [ $? -eq 0 ]; then
    echo "✅ Intel芯片版本构建成功"
else
    echo "❌ Intel芯片版本构建失败"
    exit 1
fi

# 构建M芯片版本 (ARM64)
echo "🔧 构建M芯片版本 (ARM64)..."
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o warpmini-macos-arm64 .
if [ $? -eq 0 ]; then
    echo "✅ M芯片版本构建成功"
else
    echo "❌ M芯片版本构建失败"
    exit 1
fi

# 打包Intel芯片版本
echo "📦 打包Intel芯片版本..."
zip -9 warpmini-macos-amd64.zip warpmini-macos-amd64
if [ $? -eq 0 ]; then
    echo "✅ Intel芯片版本打包成功"
else
    echo "❌ Intel芯片版本打包失败"
fi

# 打包M芯片版本
echo "📦 打包M芯片版本..."
zip -9 warpmini-macos-arm64.zip warpmini-macos-arm64
if [ $? -eq 0 ]; then
    echo "✅ M芯片版本打包成功"
else
    echo "❌ M芯片版本打包失败"
fi

# 显示文件信息
echo ""
echo "📊 构建结果："
echo "----------------------------------------"
if [ -f warpmini-macos-amd64 ]; then
    echo "Intel芯片版本: $(ls -lh warpmini-macos-amd64 | awk '{print $5}')"
    file warpmini-macos-amd64
fi

if [ -f warpmini-macos-arm64 ]; then
    echo "M芯片版本: $(ls -lh warpmini-macos-arm64 | awk '{print $5}')"
    file warpmini-macos-arm64
fi

echo ""
if [ -f warpmini-macos-amd64.zip ]; then
    echo "Intel芯片打包: $(ls -lh warpmini-macos-amd64.zip | awk '{print $5}')"
fi

if [ -f warpmini-macos-arm64.zip ]; then
    echo "M芯片打包: $(ls -lh warpmini-macos-arm64.zip | awk '{print $5}')"
fi

echo ""
echo "🎉 构建完成！"
echo ""
echo "📝 使用说明："
echo "- Intel芯片Mac用户请使用: warpmini-macos-amd64"
echo "- M系列芯片Mac用户请使用: warpmini-macos-arm64"
echo ""