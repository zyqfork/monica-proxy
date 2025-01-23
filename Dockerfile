# 使用适合Go应用的基础镜像
FROM golang:alpine AS builder
ARG TARGETOS
ARG TARGETARCH
RUN apk update && apk add --no-cache upx make && rm -rf /var/cache/apk/*

# 设置工作目录
WORKDIR /app

# 复制所有文件到容器中
COPY . .

# 下载依赖
RUN go mod tidy

# 构建应用程序
RUN make build-${TARGETOS}-${TARGETARCH}

FROM scratch AS final
WORKDIR /data
COPY --from=builder /app/build/monica /data/monica

# 开放端口
EXPOSE 8080

# 运行
CMD ["./monica"]