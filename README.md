# Epay - 支付聚合平台

基于 Go + Vue3 的 EasyPay 兼容支付聚合平台，支持支付宝和微信支付。提供管理后台和商户端，封装标准 EasyPay 协议接口，方便商户快速接入。

## 功能

**管理后台**
- 商户管理：创建、编辑、禁用/启用商户，查看商户密钥
- 订单查看：按状态、商户筛选订单，查看手续费明细
- 提现审核：审批/拒绝/确认商户提现申请
- 费率配置：设置官方费率、平台默认佣金率
- 平台配置：动态管理系统配置项

**商户端**
- 自助注册与登录
- 订单管理：查看订单列表、支付状态
- 余额查看：可用余额、冻结金额、累计收入
- 提现申请：提交提现请求，查看提现记录
- API 密钥查看：查看商户 PID 和 PKEY
- 通知地址管理：配置异步通知回调地址

**EasyPay 兼容 API**
- `POST /mapi.php` — 创建支付订单
- `GET /submit.php` — 跳转支付页面
- `POST /api.php` — 查询订单状态

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端语言 | Go 1.26 |
| Web 框架 | Gin |
| ORM | Ent |
| 数据库 | PostgreSQL 15 |
| 缓存 | Redis 7 |
| 支付 SDK | Alipay (smartwalle/alipay/v3), 微信支付 API v3 |
| 前端框架 | Vue 3 + TypeScript |
| UI 组件库 | Naive UI |
| 样式 | TailwindCSS 4 |
| 构建工具 | Vite 8 |
| 容器化 | Docker + Docker Compose |

## 快速开始

### 方式一：本地开发（推荐）

```bash
# 1. 只用 Docker 启动必要依赖：PostgreSQL + Redis
docker compose up -d
# 等价：make infra

# 2. 本机启动后端（连接上面的 Docker 依赖）
make run-local
# 后端运行在 http://localhost:8080

# 3. 另开终端启动前端
cd frontend
npm install
npm run dev
# 前端开发服务器运行在 http://localhost:5173
# 会自动代理 /api、/mapi.php 到后端 http://localhost:8080
```

> 默认 `docker compose up -d` 只启动数据库和 Redis，不启动 Go 后端和 Vue 前端。

### 方式二：完整 Docker Compose 部署

```bash
# 必须显式提供 JWT_SECRET；这会额外构建并启动 epay 后端镜像
JWT_SECRET=your-jwt-secret-at-least-32-chars-long docker compose --profile app up -d

# 访问服务
# 管理后台: http://localhost:8080/admin/login（默认账号 admin/admin123）
# 商户端:   http://localhost:8080/merchant/login
# API:      http://localhost:8080/mapi.php
# 健康检查: http://localhost:8080/health
```

> 完整 Docker 模式用于部署/验收；日常开发建议使用方式一。

### 初始化默认管理员

系统首次启动时自动创建默认管理员账号：
- 用户名：`admin`
- 密码：`admin123`（可在 `config.yaml` 的 `admin.default_password` 中修改）

首次使用建议登录管理后台后立即修改密码。

## 目录结构

```
epay/
├── cmd/server/          # 后端入口
│   └── main.go
├── internal/
│   ├── config/          # 配置加载与验证
│   ├── database/        # 数据库连接与自动迁移
│   ├── handler/
│   │   ├── admin/       # 管理后台 API 处理
│   │   ├── merchant/    # 商户端 API 处理
│   │   ├── easypay/     # EasyPay 协议处理（签名、验证、API）
│   │   └── middleware/  # JWT 认证中间件
│   ├── service/
│   │   ├── admin/       # 管理员认证与密码管理
│   │   ├── merchant/    # 商户 CRUD 与认证
│   │   ├── payment/     # 支付创建、回调处理、结算
│   │   ├── settlement/  # 提现申请、审核、结算
│   │   └── fee/         # 费率计算与商户入账
│   ├── provider/        # 支付提供商抽象层
│   │   ├── alipay/      # 支付宝（WAP/QR/页面支付）
│   │   ├── wxpay/       # 微信支付（Native/H5）
│   │   ├── registry.go  # 提供商注册表
│   │   ├── types.go     # 公共类型定义
│   │   └── provider.go  # Provider 接口定义
│   └── redis/           # Redis 客户端、分布式锁、幂等性
├── ent/                 # Ent 实体与代码生成
│   └── schema/          # 数据模型定义
│       ├── admin.go
│       ├── merchant.go
│       ├── order.go
│       ├── settlement.go
│       ├── withdraw.go
│       └── config.go
├── frontend/            # Vue 3 前端
│   └── src/
│       ├── views/admin/     # 管理后台页面
│       ├── views/merchant/  # 商户端页面
│       ├── layouts/         # 布局组件
│       ├── router/          # 路由配置
│       ├── stores/          # Pinia 状态管理
│       └── api/             # API 客户端
├── config.example.yaml  # 配置模板
├── docker-compose.yml   # Docker Compose 编排
├── Dockerfile           # 多阶段构建
└── Makefile             # 本地开发命令
```

## 配置说明

### config.yaml

```yaml
server:
  host: "0.0.0.0"     # 监听地址
  port: 8080           # 监听端口
  mode: "release"      # debug / release / test

database:
  host: "localhost"    # PostgreSQL 地址
  port: 5432           # PostgreSQL 端口
  user: "postgres"     # 数据库用户
  password: ""         # 数据库密码（生产环境建议用环境变量）
  dbname: "epay"       # 数据库名
  sslmode: "prefer"    # SSL 模式

redis:
  addr: "localhost:6379"  # Redis 地址
  password: ""            # Redis 密码
  db: 0                   # Redis 数据库编号

jwt:
  secret: "your-jwt-secret-at-least-32-chars-long"  # JWT 签名密钥
  expire_hour: 24          # Token 过期时间（小时）

admin:
  default_password: "admin123"   # 默认管理员密码

default:
  official_alipay_rate: 0.006    # 支付宝官方费率（0.6%）
  official_wxpay_rate: 0.006     # 微信支付官方费率（0.6%）
  default_platform_rate: 0.009   # 默认平台佣金率（0.9%）
```

### 环境变量覆盖

所有配置项均支持通过环境变量覆盖，规则为 `SECTION_KEY`：

| 环境变量 | 说明 |
|----------|------|
| `SERVER_HOST` | 监听地址 |
| `SERVER_PORT` | 监听端口 |
| `DATABASE_PASSWORD` | 数据库密码 |
| `REDIS_ADDR` | Redis 地址 |
| `JWT_SECRET` | JWT 签名密钥 |
| `PORT` | 端口覆盖（优先级最高） |

## API 概览

| 端点 | 说明 |
|------|------|
| `POST /api/admin/login` | 管理员登录 |
| `GET /api/admin/dashboard` | 管理后台仪表盘 |
| `GET /api/admin/merchants` | 商户列表 |
| `POST /api/admin/merchants` | 创建商户 |
| `GET /api/admin/orders` | 订单列表 |
| `GET /api/admin/withdraws` | 提现列表 |
| `POST /api/admin/withdraws/:id/approve` | 审批提现 |
| `POST /api/merchant/login` | 商户登录 |
| `POST /api/merchant/register` | 商户注册 |
| `POST /api/merchant/withdraws` | 发起提现 |
| `POST /mapi.php` | EasyPay 创建订单 |
| `GET /submit.php` | EasyPay 跳转支付 |
| `POST /api.php` | EasyPay 查询订单 |

完整 API 文档见 `docs/API.md`，EasyPay 协议文档见 `docs/EASYPAY_PROTOCOL.md`。

## 部署

```bash
# 生产部署使用 Docker Compose
docker compose up -d

# 配合 Nginx 反向代理和 HTTPS
# 详细部署指南见 docs/DEPLOY.md
```
