# Monica Proxy

这是一个代理服务，用于将 Monica 的服务转换为 ChatGPT 兼容的 API 格式。通过提供 Monica Cookie，您可以使用与 ChatGPT API
相同的接口格式来访问 Monica 的服务。

## 功能特点

- 兼容 ChatGPT API 格式
- 使用 Docker 进行简单部署
- Bearer Token 认证保护

## 快速开始

### 部署方式一：使用 Docker 运行

```bash
docker run --pull=always -d \
  --name monica-proxy \
  -p 8080:8080 \
  -e MONICA_COOKIE="你的Monica_Cookie" \
  -e BEARER_TOKEN="你想设置的验证令牌" \
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
         - BEARER_TOKEN="YOUR_BEARER_TOKEN"
   ```

   环境变量说明：
    - `MONICA_COOKIE`: Monica 的 Cookie `(格式：session_id=eyJ...)`，用于访问 Monica 的 API
    - `BEARER_TOKEN`: API 访问令牌，用于保护 API 接口安全

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

### 认证

所有 API 请求都需要在 HTTP 头部包含 Bearer Token 认证信息：

```http
Authorization: Bearer YOUR_BEARER_TOKEN
```

示例请求：

```bash
curl -X POST http://ip:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR_BEARER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [
      {
        "role": "user",
        "content": "你好"
      }
    ],
    "max_tokens": 4096, 
    "stream": true
  }'
```

## 注意事项

- 请确保 MONICA_COOKIE 和 BEARER_TOKEN 环境变量正确设置
- 确保端口未被占用
- Cookie 和 Bearer Token 应妥善保管，不要泄露给他人
- 所有 API 请求都需要提供有效的 Bearer Token