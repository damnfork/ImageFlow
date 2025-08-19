#!/bin/bash

# ImageFlow OIDC 启动脚本
# 用于OIDC认证模式

echo "正在启动 ImageFlow (OIDC模式)..."

# 设置Go环境变量，确保使用ARM64架构
export CGO_ENABLED=1
export GOARCH=arm64

# OIDC配置 - 基于 auth.ckneedu.com 的正确配置
export AUTH_TYPE=oidc
export OIDC_ISSUER=https://auth.ckneedu.com
export OIDC_CLIENT_ID=bcb42f10-f62e-4faa-910e-d7a271679e4e
export OIDC_CLIENT_SECRET=huhqgI0Bg2ku7fVwapqV06BbDfjshSbh
export OIDC_REDIRECT_URL=http://localhost:3000/auth/callback
export OIDC_SCOPES=openid,profile,email
export JWT_SIGNING_KEY=uwM6/llgdegLpsHf33IDXpSszerSY4cVRLC4JSEJ5J0=

# 服务器配置
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
    echo "认证模式: OIDC"
    echo "按 Ctrl+C 停止服务器"
    ./imageflow
else
    echo "编译失败!"
    exit 1
fi
