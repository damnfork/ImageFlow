#!/bin/bash

# ImageFlow 启动脚本
# 解决Apple Silicon Mac上的架构和OIDC配置问题

echo "正在启动 ImageFlow..."

# 设置Go环境变量，确保使用ARM64架构
export CGO_ENABLED=1
export GOARCH=arm64

# 设置应用配置，使用API Key认证避免OIDC配置问题
export AUTH_TYPE=api_key
export API_KEY=your_api_key_here
export DEBUG_MODE=true
export SERVER_ADDR=0.0.0.0:8686
export LOCAL_STORAGE_PATH=./static/images
export STORAGE_TYPE=local

# 确保静态文件目录存在
mkdir -p static/images

echo "编译程序..."
go build -o imageflow main.go

if [ $? -eq 0 ]; then
    echo "启动成功! 服务器运行在: http://localhost:8686"
    echo "API Key: your_api_key_here"
    echo "按 Ctrl+C 停止服务器"
    ./imageflow
else
    echo "编译失败!"
    exit 1
fi
