# API 文档

Epay 提供三组 API：管理后台 API、商户端 API、EasyPay 支付协议 API。所有 API 返回统一格式的 JSON 响应。

## 通用规范

### 基础 URL

```
http://<host>:8080
```

### 响应格式

成功响应：
```json
{
  "code": 0,
  "msg": "ok",
  "data": { ... }
}
```

错误响应：
```json
{
  "code": -1,
  "msg": "错误描述"
}
```

### 认证方式

管理后台和商户端 API 使用 JWT Bearer Token 认证：

```
Authorization: Bearer <token>
```

Token 通过登录接口获取，默认 24 小时过期。

---

## 一、管理后台 API

### 1.1 管理员登录

```
POST /api/admin/login
```

无需认证。使用管理员用户名和密码登录，返回 JWT Token。

**请求体：**
```json
{
  "username": "admin",
  "password": "admin123"
}
```

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "username": "admin"
  }
}
```

### 1.2 修改密码

```
POST /api/admin/change-password
```

需要认证（管理员角色）。

**请求体：**
```json
{
  "old_password": "admin123",
  "new_password": "newpass456"
}
```

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

### 1.3 仪表盘

```
GET /api/admin/dashboard
```

需要认证（管理员角色）。返回今日订单统计和待处理提现数量。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "today_order_count": 42,
    "today_revenue": 12580.50,
    "pending_withdraw_count": 3
  }
}
```

### 1.4 商户列表

```
GET /api/admin/merchants?page=1&limit=20&status=active
```

需要认证（管理员角色）。

**查询参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 否 | 页码，默认 1 |
| limit | int | 否 | 每页条数，默认 20，最大 100 |
| status | string | 否 | 筛选状态：active / disabled |

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "items": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "pid": 1001,
        "name": "测试商户",
        "fee_rate": 0.009,
        "status": "active",
        "notify_url": "https://merchant.com/notify",
        "created_at": "2025-01-01T00:00:00Z",
        "updated_at": "2025-01-01T00:00:00Z"
      }
    ],
    "total": 50,
    "page": 1,
    "limit": 20,
    "total_pages": 3
  }
}
```

> 注意：列表接口中 `pkey` 字段被隐藏（返回空字符串），需通过商户详情或密钥管理接口查看。

### 1.5 创建商户

```
POST /api/admin/merchants
```

需要认证（管理员角色）。创建商户时会自动生成唯一的 PID 和 PKEY，并创建关联的结算账户。

**请求体：**
```json
{
  "name": "新商户名称",
  "fee_rate": 0.009,
  "notify_url": "https://merchant.com/pay/notify"
}
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 商户名称 |
| fee_rate | float | 否 | 平台佣金率，默认 1.0（表示 1.0%） |
| notify_url | string | 否 | 异步通知地址 |

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "pid": 1002,
    "pkey": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
    "name": "新商户名称",
    "fee_rate": 0.009,
    "status": "active",
    "notify_url": "https://merchant.com/pay/notify",
    "created_at": "2025-01-15T00:00:00Z",
    "updated_at": "2025-01-15T00:00:00Z"
  }
}
```

### 1.6 更新商户

```
PUT /api/admin/merchants/:id
```

需要认证（管理员角色）。支持部分更新，未提供的字段保持不变。

**请求体：**
```json
{
  "name": "新名称",
  "status": "disabled",
  "fee_rate": 0.005,
  "notify_url": "https://newurl.com/notify"
}
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 否 | 商户名称 |
| status | string | 否 | 状态：active / disabled |
| fee_rate | float | 否 | 平台佣金率 |
| notify_url | string | 否 | 异步通知地址 |

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": { ... }
}
```

### 1.7 重新生成商户密钥

```
POST /api/admin/merchants/:id/regenerate-key
```

需要认证（管理员角色）。生成新的 32 位随机 PKEY，旧密钥立即失效。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "pkey": "new-a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
  }
}
```

### 1.8 订单列表

```
GET /api/admin/orders?page=1&limit=20&status=PAID&merchant_id=<uuid>
```

需要认证（管理员角色）。

**查询参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| page | int | 否 | 页码，默认 1 |
| limit | int | 否 | 每页条数，默认 20 |
| status | string | 否 | 筛选状态 |
| merchant_id | uuid | 否 | 筛选商户 |

**订单状态：** `PENDING` / `PAID` / `SETTLED` / `EXPIRED` / `CANCELLED`

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "items": [
      {
        "id": "660e8400-e29b-41d4-a716-446655440000",
        "order_no": "202501150001",
        "merchant_name": "测试商户",
        "type": "alipay",
        "amount": 100.00,
        "fee_official": 0.60,
        "fee_platform": 0.90,
        "net_amount": 99.10,
        "trade_no": "2025011522001411111111111111",
        "status": "PAID",
        "notify_url": "https://merchant.com/notify",
        "paid_at": "2025-01-15T10:30:00Z",
        "created_at": "2025-01-15T10:00:00Z"
      }
    ],
    "total": 100,
    "page": 1,
    "limit": 20,
    "total_pages": 5
  }
}
```

### 1.9 提现列表

```
GET /api/admin/withdraws?page=1&limit=20&status=PENDING
```

需要认证（管理员角色）。

**提现状态：** `PENDING` / `APPROVED` / `PAID` / `REJECTED`

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "items": [
      {
        "id": "770e8400-e29b-41d4-a716-446655440000",
        "merchant_id": "550e8400-e29b-41d4-a716-446655440000",
        "merchant_name": "测试商户",
        "amount": 5000.00,
        "account_info": "支付宝: test@alipay.com",
        "status": "PENDING",
        "remark": "",
        "created_at": "2025-01-15T08:00:00Z"
      }
    ],
    "total": 10,
    "page": 1,
    "limit": 20,
    "total_pages": 1
  }
}
```

### 1.10 审批提现

```
POST /api/admin/withdraws/:id/approve
```

需要认证（管理员角色）。将提现申请标记为 APPROVED。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

### 1.11 拒绝提现

```
POST /api/admin/withdraws/:id/reject
```

需要认证（管理员角色）。拒绝后自动解冻商户余额。

**请求体：**
```json
{
  "remark": "账户信息有误，请重新提交"
}
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| remark | string | 否 | 拒绝原因 |

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

### 1.12 确认提现

```
POST /api/admin/withdraws/:id/confirm
```

需要认证（管理员角色）。确认已付款，将 APPROVED 状态的提现标记为 PAID，更新商户结算记录（减少冻结金额，增加已提现总额）。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

### 1.13 获取平台配置

```
GET /api/admin/configs
```

需要认证（管理员角色）。返回所有平台配置项。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": [
    {
      "key": "official_alipay_rate",
      "value": "0.006",
      "description": "支付宝官方费率"
    },
    {
      "key": "official_wxpay_rate",
      "value": "0.006",
      "description": "微信支付官方费率"
    },
    {
      "key": "default_platform_rate",
      "value": "0.009",
      "description": "默认平台佣金率"
    }
  ]
}
```

### 1.14 更新平台配置

```
PUT /api/admin/configs
```

需要认证（管理员角色）。批量更新配置项，已存在的更新值，不存在的自动创建。

**请求体：**
```json
{
  "official_alipay_rate": "0.006",
  "official_wxpay_rate": "0.006",
  "default_platform_rate": "0.010"
}
```

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

---

## 二、商户端 API

### 2.1 商户注册

```
POST /api/merchant/register
```

无需认证。商户自助注册，返回 PID 和 PKEY。

**请求体：**
```json
{
  "name": "我的商店",
  "password": "mypassword"
}
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 商户名称 |
| password | string | 是 | 登录密码 |

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "pid": 1003,
    "pkey": "b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7"
  }
}
```

> 注意：PKEY 仅在注册时返回一次，请妥善保管。遗失后需联系管理员重新生成。

### 2.2 商户登录

```
POST /api/merchant/login
```

无需认证。使用 PID 和密码登录，返回 JWT Token。

**请求体：**
```json
{
  "pid": "1003",
  "password": "mypassword"
}
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| pid | string | 是 | 商户 PID |
| password | string | 是 | 登录密码 |

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "pid": 1003
  }
}
```

### 2.3 获取商户信息

```
GET /api/merchant/profile
```

需要认证（商户角色）。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "pid": 1003,
    "name": "我的商店",
    "fee_rate": 0.009,
    "status": "active",
    "notify_url": "",
    "created_at": "2025-01-15T00:00:00Z",
    "updated_at": "2025-01-15T00:00:00Z"
  }
}
```

### 2.4 获取余额

```
GET /api/merchant/balance
```

需要认证（商户角色）。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "balance": 12580.50,
    "frozen": 5000.00,
    "total_income": 50000.00,
    "total_withdrawn": 32419.50
  }
}
```

| 字段 | 说明 |
|------|------|
| balance | 可用余额 |
| frozen | 提现中冻结金额 |
| total_income | 累计收入 |
| total_withdrawn | 累计已提现 |

### 2.5 商户订单列表

```
GET /api/merchant/orders?page=1&limit=20&status=PAID
```

需要认证（商户角色）。仅返回当前商户的订单。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "items": [
      {
        "id": "660e8400-e29b-41d4-a716-446655440000",
        "order_no": "202501150001",
        "type": "alipay",
        "amount": 100.00,
        "fee_official": 0.60,
        "fee_platform": 0.90,
        "net_amount": 99.10,
        "trade_no": "2025011522001411111111111111",
        "status": "PAID",
        "notify_url": "https://merchant.com/notify",
        "paid_at": "2025-01-15T10:30:00Z",
        "created_at": "2025-01-15T10:00:00Z"
      }
    ],
    "total": 50,
    "page": 1,
    "limit": 20,
    "total_pages": 3
  }
}
```

### 2.6 商户提现列表

```
GET /api/merchant/withdraws?page=1&limit=20
```

需要认证（商户角色）。仅返回当前商户的提现记录。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "items": [
      {
        "id": "770e8400-e29b-41d4-a716-446655440000",
        "amount": 5000.00,
        "account_info": "支付宝: test@alipay.com",
        "status": "PENDING",
        "remark": "",
        "created_at": "2025-01-15T08:00:00Z"
      }
    ],
    "total": 5,
    "page": 1,
    "limit": 20,
    "total_pages": 1
  }
}
```

### 2.7 发起提现

```
POST /api/merchant/withdraws
```

需要认证（商户角色）。发起提现后系统自动冻结对应余额，待管理员审核。

**请求体：**
```json
{
  "amount": 5000.00,
  "account_info": "支付宝: test@alipay.com"
}
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| amount | float | 是 | 提现金额，必须为正数 |
| account_info | string | 是 | 收款账户信息 |

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "id": "770e8400-e29b-41d4-a716-446655440000",
    "amount": 5000.00,
    "account_info": "支付宝: test@alipay.com",
    "status": "PENDING",
    "created_at": "2025-01-15T08:00:00Z"
  }
}
```

### 2.8 获取 API 密钥

```
GET /api/merchant/api-key
```

需要认证（商户角色）。查看当前商户的 PID 和完整 PKEY。

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "pid": 1003,
    "pkey": "b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7"
  }
}
```

### 2.9 更新通知地址

```
PUT /api/merchant/notify-url
```

需要认证（商户角色）。

**请求体：**
```json
{
  "notify_url": "https://myshop.com/pay/callback"
}
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| notify_url | string | 是 | 新的异步通知地址 |

**响应：**
```json
{
  "code": 0,
  "msg": "ok",
  "data": { ... }
}
```

---

## 三、通用

### 3.1 健康检查

```
GET /health
```

无需认证。

**响应：**
```json
{
  "status": "ok"
}
```

### 3.2 错误码说明

| code | 说明 |
|------|------|
| 0 | 成功 |
| -1 | 通用业务错误 |
| 400 | 请求参数错误 |
| 401 | 未认证或 Token 过期 |
| 403 | 权限不足 |

### 3.3 错误消息示例

```json
// 认证失败
{ "code": 401, "msg": "invalid or expired token" }

// 权限不足（商户调用管理接口）
{ "code": 403, "msg": "admin role required" }

// 参数校验失败
{ "code": -1, "msg": "amount must be positive" }
```

---

## 相关文档

- `EASYPAY_PROTOCOL.md` — EasyPay 支付协议对接文档（面向商户）
- `DEPLOY.md` — 部署指南
- `README.md` — 项目总览与快速开始
