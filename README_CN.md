# WarpMini

一个简洁的 Warp 客户端，用于快速管理 Cloudflare Warp 登录凭据。

## 功能特性

- 🖥️ 简洁的 GUI 界面，易于使用
- 🌐 跨平台支持 (macOS, Windows, Linux)
- 🔐 自动处理 refresh_token 刷新和存储
- ⚡ 基于 Fyne 框架，原生性能
- 🔄 支持机器码刷新功能
- 🧹 一键清理登录数据

## 下载安装

### 从 Releases 下载

从 [Releases](https://github.com/yourusername/warpmini/releases) 页面下载适合你平台的最新版本：

- **macOS**: `warpmini-darwin-universal.tar.gz` (支持 Intel 和 Apple Silicon)
- **Windows**: `warpmini-windows-amd64.zip` (64位) 或 `warpmini-windows-386.zip` (32位)
- **Linux**: `warpmini-linux-amd64.tar.gz` (x64) 或 `warpmini-linux-arm64.tar.gz` (ARM64)

### 从源码构建

```bash
git clone https://github.com/yourusername/warpmini.git
cd warpmini
go build -o warpmini ./cmd/warpmini
```

## 使用方法

1. 解压下载的文件并运行 `warpmini`
2. 在文本框中输入你的 `refresh_token`
3. 选择是否在登录前刷新机器码（推荐勾选）
4. 点击「登录」按钮进行认证
5. 应用会自动处理凭据存储并启动 Warp 客户端

### 获取 refresh_token

你可以通过以下方式获取 refresh_token：
- 使用其他 Warp 工具导出
- 从现有的 Warp 配置文件中提取
- 通过 Warp 官方 API 获取

## 开发构建

### 环境要求

- Go 1.21 或更高版本
- CGO 支持的 C 编译器
- 各平台的 GUI 开发库

### 本地构建

```bash
# 基本构建
go build -o warpmini ./cmd/warpmini

# 优化构建
go build -ldflags="-s -w" -o warpmini ./cmd/warpmini
```

### 多平台构建

本项目使用 GitHub Actions 进行自动化多平台构建。查看 `.github/workflows/build.yml` 了解详情。

## 项目结构

```
warpmini/
├── cmd/warpmini/          # 主程序入口
├── internal/platform/     # 平台特定功能
├── pkg/theme/            # UI 主题配置
├── assets/               # 静态资源
└── .github/workflows/    # GitHub Actions 配置
```

## 贡献指南

欢迎提交 Issue 和 Pull Request！

## 许可证

本项目基于 MIT 许可证开源 - 查看 [LICENSE](LICENSE) 文件了解详情。