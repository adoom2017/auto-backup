# 使用golang官方镜像作为构建环境
FROM golang:1.22-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装构建必需的包
RUN apk add --no-cache gcc musl-dev

# 复制go.mod和go.sum文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=1 GOOS=linux go build -o auto-backup .

# 使用alpine作为运行环境
FROM alpine:latest

# 安装ca证书和sqlite3
RUN apk --no-cache add ca-certificates sqlite

WORKDIR /root/

# 从builder阶段复制二进制文件
COPY --from=builder /app/auto-backup .

# 创建必要的目录
RUN mkdir -p logs backups config

# 复制配置文件
COPY config_example.yaml ./config.yaml

# 暴露端口
EXPOSE 8080

# 运行应用
CMD ["./auto-backup"]
