# 部署指南

## 环境要求

- Docker 24+ 和 Docker Compose v2+
- 服务器 2 核 4GB 以上配置
- 域名（用于 HTTPS 和支付回调）
- 支付宝商户号和微信支付商户号

## 端口规划

| 服务 | 端口 | 说明 |
|------|------|------|
| epay API | 8080 | 后端 API 和支付接口 |
| PostgreSQL | 5432 | 数据库 |
| Redis | 6379 | 缓存 |
| 管理后台 (前端) | 3002 | 管理后台页面 |
| 商户端 (前端) | 3001 | 商户自助页面 |

---

## 方式一：Docker Compose 部署（推荐）

### 1. 准备配置文件

```bash
# 创建工作目录
mkdir -p /opt/epay
cd /opt/epay

# 复制项目文件
# 将项目文件复制到 /opt/epay 目录

# 创建配置
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，修改关键配置项：

```yaml
jwt:
  secret: "请替换为64位以上的随机字符串"  # 至少32字符
  expire_hour: 24

admin:
  default_password: "请替换为强密码"

database:
  password: "请替换为数据库密码"
```

### 2. 启动服务

```bash
docker compose up -d

# 查看日志
docker compose logs -f

# 确认服务状态
docker compose ps
```

### 3. 验证部署

```bash
# 健康检查
curl http://localhost:8080/health
# 返回: {"status":"ok"}

# 测试 API
curl -X POST http://localhost:8080/api/admin/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

---

## 方式二：裸机部署

### 1. 安装依赖

```bash
# PostgreSQL 15
apt install postgresql-15

# Redis 7
apt install redis-server

# Go 1.26+
wget https://go.dev/dl/go1.26.x.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.26.x.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Node.js 20+
curl -fsSL https://deb.nodesource.com/setup_20.x | bash
apt install nodejs
```

### 2. 构建后端

```bash
# 编译
make build
# 二进制文件生成到 bin/epay

# 复制配置
cp config.example.yaml config.yaml
# 编辑 config.yaml 配置数据库连接
```

### 3. 构建前端

```bash
cd frontend

# 安装依赖
npm install

# 构建生产版本
npm run build
# 构建产物在 frontend/dist

# 将 dist 部署到 Nginx 或 CDN
```

### 4. 启动后端

```bash
# 使用 systemd 管理
cat > /etc/systemd/system/epay.service << 'EOF'
[Unit]
Description=Epay Payment Platform
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=epay
WorkingDirectory=/opt/epay
ExecStart=/opt/epay/bin/epay
Restart=always
RestartSec=10
Environment=GIN_MODE=release
Environment=DATABASE_PASSWORD=your_password
Environment=JWT_SECRET=your_jwt_secret

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable epay
systemctl start epay
```

---

## Nginx 反向代理配置

### 基础配置

```nginx
server {
    listen 80;
    server_name pay.yourdomain.com;

    # 后端 API
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 支付回调可能需要更长的超时
        proxy_read_timeout 60s;
        proxy_connect_timeout 10s;
    }

    # 静态资源缓存
    location /assets {
        proxy_pass http://127.0.0.1:8080;
        expires 7d;
        add_header Cache-Control "public, immutable";
    }
}
```

### 管理后台和商户端前端

如果使用独立域名部署前端：

```nginx
# 管理后台
server {
    listen 80;
    server_name admin.yourdomain.com;

    root /opt/epay/frontend/admin-dist;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }
}

# 商户端
server {
    listen 80;
    server_name merchant.yourdomain.com;

    root /opt/epay/frontend/merchant-dist;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    # 代理 API 请求到后端
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## HTTPS 配置（Let's Encrypt）

```bash
# 安装 certbot
apt install certbot python3-certbot-nginx

# 申请证书（自动配置 Nginx）
certbot --nginx -d pay.yourdomain.com

# 自动续期
certbot renew --dry-run
```

Nginx HTTPS 配置示例：

```nginx
server {
    listen 443 ssl http2;
    server_name pay.yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/pay.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/pay.yourdomain.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # 后端代理配置同 HTTP
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

# HTTP 重定向到 HTTPS
server {
    listen 80;
    server_name pay.yourdomain.com;
    return 301 https://$server_name$request_uri;
}
```

---

## 支付回调配置

### 支付宝

1. 在支付宝开放平台配置回调地址：
   - 异步通知地址：`https://pay.yourdomain.com/api/alipay/notify`
   - 同步返回地址：`https://pay.yourdomain.com/order/result`

2. 在 `config.yaml` 中配置支付宝凭证：

```yaml
alipay:
  app_id: "你的支付宝应用ID"
  private_key: |
    -----BEGIN PRIVATE KEY-----
    ...
    -----END PRIVATE KEY-----
  public_key: |
    -----BEGIN PUBLIC KEY-----
    ...
    -----END PUBLIC KEY-----
  notify_url: "https://pay.yourdomain.com/api/alipay/notify"
  return_url: "https://pay.yourdomain.com/order/result"
```

### 微信支付

1. 在微信支付商户平台配置回调地址：
   - 支付通知地址：`https://pay.yourdomain.com/api/wxpay/notify`

2. 在 `config.yaml` 中配置微信支付凭证：

```yaml
wxpay:
  app_id: "你的微信应用AppID"
  mch_id: "你的微信商户号"
  private_key: |
    -----BEGIN PRIVATE KEY-----
    ...
    -----END PRIVATE KEY-----
  apiv3_key: "32字节的APIv3密钥"
  public_key: |
    -----BEGIN PUBLIC KEY-----
    ...
    -----END PUBLIC KEY-----
  public_key_id: "微信支付平台公钥ID"
  serial_no: "商户证书序列号"
  notify_url: "https://pay.yourdomain.com/api/wxpay/notify"
```

---

## 环境变量

可在 `docker-compose.yml` 或 systemd 中设置，优先级高于 `config.yaml`：

| 环境变量 | 说明 | 示例 |
|----------|------|------|
| `PORT` | 服务端口 | `8080` |
| `GIN_MODE` | Gin 运行模式 | `release` |
| `SERVER_HOST` | 监听地址 | `0.0.0.0` |
| `DATABASE_HOST` | 数据库地址 | `localhost` |
| `DATABASE_PORT` | 数据库端口 | `5432` |
| `DATABASE_USER` | 数据库用户 | `epay` |
| `DATABASE_PASSWORD` | 数据库密码 | `your_password` |
| `DATABASE_NAME` | 数据库名 | `epay` |
| `DATABASE_SSLMODE` | 数据库 SSL 模式 | `disable` |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `REDIS_PASSWORD` | Redis 密码 | `` |
| `REDIS_DB` | Redis 库编号 | `0` |
| `JWT_SECRET` | JWT 签名密钥 | `64位随机字符串` |

---

## 常见问题

### 1. 数据库连接失败

检查 `config.yaml` 或环境变量中的数据库配置是否正确。使用 Docker Compose 时确保 postgres 服务已就绪：

```bash
docker compose logs postgres
```

### 2. 支付回调收不到

- 确保服务器有公网 IP，域名 DNS 解析正确
- 检查 Nginx 是否正常代理 `/api/alipay/notify` 和 `/api/wxpay/notify`
- 查看 epay 日志：
```bash
docker compose logs epay
```

### 3. 前端页面空白

- 检查 Vite 构建是否成功
- 检查 Nginx 静态资源路径配置
- 浏览器控制台查看是否有 API 请求错误

### 4. 数据库自动迁移

Epay 首次启动时自动执行数据库迁移（创建表结构）。不需要手动执行 SQL。如果迁移失败，检查数据库用户是否有建表权限。

---

## 生产环境检查清单

- [ ] JWT Secret 已改为64位以上随机字符串
- [ ] 管理员默认密码已修改
- [ ] 数据库密码已设置强密码
- [ ] HTTPS 已配置
- [ ] 支付宝/微信支付凭证已配置
- [ ] 支付回调地址已配置且公网可达
- [ ] PostgreSQL data 目录已备份策略
- [ ] 服务器防火墙已开放 443 端口
- [ ] 日志轮转已配置
- [ ] 监控告警已设置
