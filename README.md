# Monica Proxy

这是一个代理服务，用于将 Monica 的服务转换为 ChatGPT 兼容的 API 格式。通过提供 Monica Cookie，您可以使用与 ChatGPT API 相同的接口格式来访问 Monica 的服务。

## 功能特点

- 兼容 ChatGPT API 格式
- 使用 Docker 进行简单部署

## 快速开始

### 部署方式一：使用 Docker 运行

```bash
docker run -d \
  --name monica-proxy \
  -p 8080:8080 \
  -e MONICA_COOKIE=你的Monica_Cookie \
  neccen/monica-proxy:latest
```

### 部署方式二：使用 Docker Compose

1. 创建 `docker-compose.yml` 文件：
   ```yaml
   services:
     monica-proxy:
       image: neccen/monica-proxy:latest
       container_name: monica-proxy
       restart: unless-stopped
       ports:
         - "8080:8080"
       environment:
         - MONICA_COOKIE="MONICA_COOKIE"
   ```

   - `MONICA_COOKIE`: Monica 的 Cookie，用于访问 Monica 的 API

2. 启动服务
   ```bash
   docker-compose up -d
   ```

服务将在 `http://ip:8080/v1/chat/completions` 上运行。

## API 使用

兼容 ChatGPT API 格式，地址为：
```
http://ip:8080/v1/chat/completions
```

## 注意事项

- 请确保 MONICA_COOKIE 环境变量正确设置
- 确保端口未被占用
- Cookie 应妥善保管，不要泄露给他人