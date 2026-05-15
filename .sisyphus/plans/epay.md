# Epay 支付聚合平台 — 工作规划

## TL;DR

> **Quick Summary**: 构建 EasyPay 协议兼容的支付聚合平台（Go+Gin+Vue3），管理员接入支付宝/微信后暴露标准 EasyPay API 给商户系统调用，支持费率自动扣减、余额管理和人工提现审核。
>
> **Deliverables**:
> - Go 后端：EasyPay 协议 API + 支付宝/微信 Provider + 管理/商户 API
> - Vue3 前端：管理后台（NaiveUI）+ 商户端（NaiveUI）
> - 数据库：PostgreSQL + Redis
> - 部署：Docker Compose 一键启动
>
> **Estimated Effort**: Large
> **Parallel Execution**: YES — 5 waves
> **Critical Path**: 项目脚手架 → 数据库 → Provider 接口 → 支付宝/微信实现 → EasyPay API → 前端 → 集成测试

---

## Context

### Original Request
用户需要构建一个类似易支付的分账平台，管理员接入支付宝和微信支付，然后对外暴露 EasyPay 兼容接口给商户系统调用。参考 sub2api 的支付实现。

### Interview Summary
**Key Discussions**:
- 技术栈：Go + Gin + Ent、PostgreSQL、Redis、Vue3 + NaiveUI + Tailwind
- 三个端口：管理后台（自己用）、商户端（商户用）、EasyPay API（商户系统调用）
- 费率模型：官方费率（0.6% 默认）+ 平台费率（全局可配 + 商户可覆盖），商户结算 = 订单金额 - 平台费
- 结算方式：商户申请提现 → 管理员人工审核/打款
- 用户将依赖 AI 维护此项目，自己看不懂 Go 代码
- 前端最终选择 Naive UI（颜值与功能平衡，暗黑模式支持好）

**Research Findings**:
- sub2api (`C:\Users\zhengjiyong\code\sub2api`) 有完整的 Payment Provider 接口和支付宝/微信/EasyPay 实现可供参考
- EasyPay 协议：三个端点（submit.php / mapi.php / api.php），MD5 签名，标准回调参数
- 支付宝 SDK：`smartwalle/alipay/v3`；微信支付：APIv3 + PEM 证书
- 回调通知必须返回纯文本 `"success"`，不能有查询参数

### Metis Review
**Identified Gaps** (incorporated into plan):
- **Provider 快照**：创单时必须保存支付配置快照（JSONB），回调时交叉验证防篡改 → 已纳入 Task 7
- **重复订单检测**：同一 `out_trade_no` 参数变化时拒绝 → 已纳入 Task 6
- **notify_url 校验**：禁止包含查询参数，禁止内网地址（防 SSRF）→ 已纳入 Task 6
- **回调重试机制**：10s→60s→10min→30min→60min，最少 5 次 → 已纳入 Task 8
- **api.php 密钥暴露**：查询接口使用 `pid+key` 明文传输，协议级设计 → 文档说明

---

## Work Objectives

### Core Objective
构建一个可独立部署的 EasyPay 兼容支付聚合平台，管理员配置支付宝/微信后，商户通过标准 EasyPay 协议接入完成支付，平台自动计算费率并管理余额。

### Concrete Deliverables
- Go 后端二进制（含所有 API 和 Provider）
- Vue3 管理后台 SPA
- Vue3 商户端 SPA
- Docker Compose 部署配置
- 项目 README 和 API 文档

### Definition of Done
- [ ] `docker compose up -d` 一键启动全部服务
- [ ] `/mapi.php` 可成功创建支付宝和微信支付订单
- [ ] 支付回调正确入账、扣费率、增加商户余额
- [ ] 管理后台可创建商户、配置费率、审核提现
- [ ] 商户端可查看订单、查看余额、申请提现

### Must Have
- EasyPay 协议完全兼容（sign 验签、回调格式、返回码）
- 支付宝当面付（扫码）和微信 Native 支付
- 费率自动扣除（官方 + 平台），商户余额实时更新
- 提现申请 + 审核流程
- Provider 配置快照防篡改
- Docker Compose 部署

### Must NOT Have (Guardrails)
- **NOT** 自动打款到商户支付宝/微信（人工处理）
- **NOT** Stripe / PayPal / 国际支付（只做支付宝+微信）
- **NOT** 订阅/周期扣款
- **NOT** 退款功能（首版不做，后续可加）
- **NOT** 多管理员 / RBAC 权限系统
- **NOT** 允许 notify_url 包含查询参数
- **NOT** 允许 notify_url 指向内网 IP（防 SSRF）
- **NOT** AI slop：过度抽象、无意义工具类、超长 JSDoc

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed.

### Test Decision
- **Infrastructure exists**: NO（新项目从零开始）
- **Automated tests**: NO（首版手动 QA，后续可加）
- **Framework**: N/A

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **Backend/API**: Bash（curl）— 发送请求，断言状态码 + 响应字段
- **Frontend/UI**: Playwright — 导航、填表、点击、断言 DOM、截图
- **CLI/Build**: Bash — 编译、启动、Docker 操作

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — 基础设施):
├── Task 1: 项目脚手架 — Go 模块、目录结构、Dockerfile
├── Task 2: 数据库 Schema — Ent schemas + 迁移
├── Task 3: Redis 集成 — 连接池、回调幂等键
└── Task 4: 配置管理 — 环境变量/config 加载

Wave 2 (After Wave 1 — 支付核心, MAX PARALLEL):
├── Task 5: Provider 接口定义 — Provider/CancelableProvider 接口 + 类型
├── Task 6: EasyPay 协议处理器 — /mapi.php /submit.php /api.php (验签+路由)
├── Task 7: 支付宝 Provider — 扫码/跳转支付 (参考 sub2api)
└── Task 8: 微信支付 Provider — Native/H5 支付 (参考 sub2api)

Wave 3 (After Wave 2 — 业务层):
├── Task 9: 支付 Service — 订单创建/查询/回调处理/重复检测
├── Task 10: 费率 Service — 费率计算、Provider 快照保存
├── Task 11: 结算 Service — 余额管理、提现申请/审核
├── Task 12: 商户 Service — 商户 CRUD、密钥管理

Wave 4 (After Wave 1 — 管理后台, 可提前开始):
├── Task 13: Vue3 项目搭建 — Vite + NaiveUI + Tailwind + 路由
├── Task 14: 管理后台页面 — 登录/商户管理/订单/提现审核/配置
├── Task 15: 商户端页面 — 注册/登录/订单/余额/提现/密钥

Wave 5 (After Wave 3+4 — 集成部署):
├── Task 16: Admin API — 管理后台后端接口
├── Task 17: Merchant API — 商户端后端接口
├── Task 18: Docker Compose — 完整部署配置
├── Task 19: 集成测试 — 端到端支付流程验证
├── Task 20: 文档 — README、API 文档、部署指南
```

**Critical Path**: Task 1 → Task 2 → Task 5 → Task 6/7/8 → Task 9 → Task 16/17 → Task 18 → Task 19
**Parallel Speedup**: Waves 2 和 4 可并行，整体 ~40% 加速
**Max Concurrent**: Wave 2 有 4 个独立任务

### Agent Dispatch Summary

- **Wave 1**: 4 — T1-T4 → `quick` / `unspecified-high`
- **Wave 2**: 4 — T5-T8 → `unspecified-high` / `deep`
- **Wave 3**: 4 — T9-T12 → `deep` / `unspecified-high`
- **Wave 4**: 3 — T13-T15 → `visual-engineering` / `quick`
- **Wave 5**: 5 — T16-T20 → `unspecified-high` / `unspecified-low` / `writing`

---

## TODOs

- [x] 1. 项目脚手架 — Go 模块、目录结构、Dockerfile

  **What to do**:
  - 初始化 Go module：`go mod init epay`
  - 创建目录结构：`cmd/server/`、`internal/{config,handler,service,model,provider}`、`frontend/{admin,merchant}`
  - 创建 `cmd/server/main.go` 入口（Gin 路由骨架 + 健康检查）
  - 创建 `Dockerfile`（多阶段构建：Go 编译 + 静态文件嵌入）
  - 创建 `docker-compose.yml` 骨架（Go + PostgreSQL + Redis）
  - 创建 `Makefile`（build/run/dev 目标）

  **Must NOT do**:
  - 不要写任何业务逻辑
  - 不要引入不必要的第三方库（只加 gin、ent、redis 基础依赖）

  **Recommended Agent Profile**:
  > Select category + skills based on task domain.
  - **Category**: `quick`
    - Reason: 纯脚手架搭建，单文件操作，无复杂逻辑
  - **Skills**: [`go`, `gin`]
    - 标准目录结构遵循 Go 社区规范

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3, 4)
  - **Blocks**: Task 5 (Provider 接口), Task 13 (前端搭建)
  - **Blocked By**: None

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\cmd\server\` — 入口结构参考
  - `C:\Users\zhengjiyong\code\sub2api\deploy\docker-compose.yml` — Docker Compose 模板

  **Acceptance Criteria**:
  - [ ] `go build ./cmd/server/` 编译成功
  - [ ] `curl http://localhost:8080/health` 返回 200
  - [ ] `docker compose up -d` 启动 Go+PG+Redis 三容器
  - [ ] `docker compose down` 正常停止

  **QA Scenarios**:

  ```
  Scenario: 项目编译并启动
    Tool: Bash
    Preconditions: 无
    Steps:
      1. cd 到项目根目录
      2. go build -o epay.exe ./cmd/server/
      3. 启动编译产物
      4. curl http://localhost:8080/health
    Expected Result: HTTP 200 + body 包含 "ok"
    Failure Indicators: 编译错误、启动失败、健康检查非 200
    Evidence: .sisyphus/evidence/task-1-build-start.txt

  Scenario: Docker Compose 全栈启动
    Tool: Bash
    Preconditions: Docker Desktop 运行中
    Steps:
      1. docker compose up -d
      2. docker compose ps
      3. curl http://localhost:8080/health
    Expected Result: 三容器均为 Up/healthy 状态
    Failure Indicators: 容器异常退出、健康检查失败
    Evidence: .sisyphus/evidence/task-1-docker-up.txt
  ```

  **Commit**: YES
  - Message: `chore: project scaffolding with Go module, Docker setup`
  - Files: `go.mod`, `go.sum`, `cmd/`, `Dockerfile`, `docker-compose.yml`, `Makefile`

- [x] 2. 数据库 Schema — Ent schemas + 迁移

  **What to do**:
  - 初始化 Ent：`go run entgo.io/ent/cmd/ent new Merchant Order Settlement Withdraw Config`
  - 定义 `Merchant` schema：pid(int,unique)、pkey(string)、name、fee_rate(decimal)、status(enum:active/disabled)、notify_url
  - 定义 `Order` schema：order_no(string,unique)、merchant_id(fk)、type(enum:alipay/wxpay)、amount(decimal)、fee_official(decimal)、fee_platform(decimal)、net_amount(decimal)、trade_no、status(enum:PENDING/PAID/SETTLED/EXPIRED/CANCELLED)、notify_url、provider_snapshot(JSONB)、paid_at(timestamp)、timestamps
  - 定义 `Settlement` schema：merchant_id(fk,unique)、balance(decimal)、frozen(decimal)、total_income(decimal)、total_withdrawn(decimal)
  - 定义 `Withdraw` schema：merchant_id(fk)、amount(decimal)、account_info(text)、status(enum:PENDING/APPROVED/PAID/REJECTED)、remark、timestamps
  - 定义 `Config` schema：key(string,unique)、value(text)、description
  - 定义 `Admin` schema：username(unique)、password_hash、timestamps
  - 运行 `go generate ./ent` 生成代码
  - 创建数据库连接初始化函数

  **Must NOT do**:
  - 不要创建无关的表（退款、订阅、日志单独处理）
  - 不要添加复杂的索引，首版保持简单

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: Ent 代码生成需要理解 ORM schema 定义
  - **Skills**: [`go`, `ent`, `database`]

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3, 4)
  - **Blocks**: Tasks 6-12 (所有业务层)
  - **Blocked By**: Task 1 (go.mod 存在)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\ent\schema\` — Ent schema 定义参考
  - `https://entgo.io/docs/schema-fields` — Ent 官方 schema 文档
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\types.go` — OrderStatus/PaymentType 常量

  **Acceptance Criteria**:
  - [ ] `go generate ./ent` 无错误
  - [ ] 所有 schema 文件定义完整
  - [ ] 数据库连接函数可正常连接 PostgreSQL

  **QA Scenarios**:

  ```
  Scenario: Schema 生成和迁移
    Tool: Bash
    Preconditions: PostgreSQL 运行中
    Steps:
      1. go generate ./ent
      2. go run ./cmd/migrate（或等效的迁移命令）
      3. 用 psql 或 ent client 查询表列表
    Expected Result: 7 张表创建成功，无错误
    Failure Indicators: 生成失败、迁移报错、表缺失
    Evidence: .sisyphus/evidence/task-2-schema.txt

  Scenario: 数据库连接正常
    Tool: Bash
    Preconditions: Schema 已迁移
    Steps:
      1. 启动服务
      2. curl http://localhost:8080/health（含 DB check）
    Expected Result: 健康检查确认 DB 连接正常
    Failure Indicators: DB 连接超时或拒绝
    Evidence: .sisyphus/evidence/task-2-db-check.txt
  ```

  **Commit**: YES
  - Message: `feat: ent schemas for core domain models`
  - Files: `ent/schema/*.go`, `internal/database/*.go`

- [x] 3. Redis 集成 — 连接池、回调幂等工具

  **What to do**:
  - 安装 go-redis 依赖
  - 创建 Redis 连接初始化（从配置读取 host/port/password/db）
  - 实现幂等键工具：`SetNX(key, ttl)` 防止回调重复处理
  - 实现简单分布式锁：`Lock(key, ttl)` / `Unlock(key)`
  - 在健康检查中加入 Redis ping

  **Must NOT do**:
  - 不要实现队列/消息系统（首版不需要）
  - 不要引入 Redisson 等重型框架

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准 Redis 集成，无复杂逻辑
  - **Skills**: [`go`, `redis`]

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 4)
  - **Blocks**: Tasks 9 (支付回调去重), 11 (结算锁)
  - **Blocked By**: Task 1 (go.mod)

  **References**:
  - `https://redis.uptrace.dev/guide/go-redis.html` — go-redis 官方文档
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\repository\` — Redis 缓存模式参考

  **Acceptance Criteria**:
  - [ ] `redis.Ping()` 成功
  - [ ] `SetNX` 幂等键功能正确（第二次 SetNX 返回 false）
  - [ ] `Lock/Unlock` 分布式锁正确获取和释放

  **QA Scenarios**:

  ```
  Scenario: Redis 连接和基本操作
    Tool: Bash
    Preconditions: Redis 运行中
    Steps:
      1. 启动服务，检查 /health 返回 Redis: OK
      2. 用 redis-cli 验证连接
    Expected Result: Redis ping 成功
    Failure Indicators: 连接超时、密码错误
    Evidence: .sisyphus/evidence/task-3-redis-health.txt

  Scenario: 幂等键防重复
    Tool: Bash (curl)
    Preconditions: 服务运行中
    Steps:
      1. 调用一个需要幂等的测试端点
      2. 用相同参数再次调用
      3. 第二次调用返回 "duplicate" 或幂等结果
    Expected Result: 第二次调用被拦截
    Failure Indicators: 两次调用都成功处理
    Evidence: .sisyphus/evidence/task-3-idempotent.txt
  ```

  **Commit**: YES
  - Message: `feat: redis integration with idempotency and lock utilities`
  - Files: `internal/redis/*.go`

- [x] 4. 配置管理 — 环境变量/config 加载

  **What to do**:
  - 安装 viper 依赖
  - 定义配置结构体：Server、Database、Redis、Alipay、Wxpay、Default（费率等）
  - 创建 `config.example.yaml` 完整配置模板
  - 配置加载优先级：环境变量 > config.yaml > 默认值
  - 敏感字段支持环境变量覆盖（如 DB 密码）
  - 创建 `internal/config/config.go` 单例

  **Must NOT do**:
  - 不要硬编码任何密钥/密码
  - 不要将 config.example.yaml 中的示例密钥当真

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: 标准配置加载，viper 模式化操作
  - **Skills**: [`go`, `viper`]

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 3)
  - **Blocks**: 所有后续任务（都需要配置）
  - **Blocked By**: Task 1 (go.mod)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\deploy\config.example.yaml` — 配置结构参考
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\config\config.go` — 配置加载参考
  - `https://github.com/spf13/viper` — Viper 官方文档

  **Acceptance Criteria**:
  - [ ] `config.Load()` 能从 yaml 文件加载所有字段
  - [ ] 环境变量可覆盖 yaml 中的值
  - [ ] 缺少必填字段时启动报错（不是 nil panic）

  **QA Scenarios**:

  ```
  Scenario: 配置文件正常加载
    Tool: Bash
    Preconditions: config.yaml 存在
    Steps:
      1. 启动服务
      2. 检查日志输出配置摘要（无敏感信息）
    Expected Result: 配置加载成功，日志输出非敏感配置
    Failure Indicators: 启动失败、配置字段为空
    Evidence: .sisyphus/evidence/task-4-config-load.txt

  Scenario: 环境变量覆盖
    Tool: Bash
    Preconditions: 无
    Steps:
      1. 设置 DB_HOST=override-host
      2. 启动服务
      3. 验证 DB 连接使用 override-host
    Expected Result: 环境变量值生效
    Failure Indicators: 仍使用 yaml 中的旧值
    Evidence: .sisyphus/evidence/task-4-env-override.txt
  ```

  **Commit**: YES
  - Message: `feat: configuration management with viper`
  - Files: `internal/config/*.go`, `config.example.yaml`

- [x] 5. Provider 接口定义 — Payment Provider 抽象层

  **What to do**:
  - 定义 `Provider` 接口：`Name()`、`ProviderKey()`、`SupportedTypes()`、`CreatePayment()`、`QueryOrder()`、`VerifyNotification()`
  - 定义 `CreatePaymentRequest`、`CreatePaymentResponse`、`QueryOrderResponse`、`PaymentNotification` 等 DTO
  - 定义 `PaymentType` 常量：`alipay`、`wxpay`、`alipay_direct`、`wxpay_direct`
  - 定义 `ProviderStatus` 常量：`pending`、`paid`、`success`、`failed`
  - 定义 `OrderStatus` 常量：`PENDING`、`PAID`、`SETTLED`、`EXPIRED`、`CANCELLED`
  - 创建 Provider 注册表（registry）：`Register(key, factory)` / `Get(key)` / `List()`
  - Provider 工厂函数：接收 instanceID + config map，返回 Provider 实例

  **Must NOT do**:
  - 不要包含具体支付实现（支付宝/微信实现在 Task 7/8）
  - 不要定义 EasyPay 协议相关的类型（那是 handler 层的）

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 接口设计影响所有后续实现，需要深思熟虑
  - **Skills**: [`go`, `architecture`]

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 6, 7, 8 — 但 T7/T8 依赖于 T5)
  - **Blocks**: Tasks 7, 8 (支付宝/微信实现)
  - **Blocked By**: Task 1 (go.mod), Task 2 (OrderStatus 常量)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\types.go` — **MUST READ**：完整的 Provider 接口、DTO 定义、状态常量
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\registry.go` — Provider 注册表模式

  **Acceptance Criteria**:
  - [ ] `Provider` 接口包含 4 个方法（不含 Refund）
  - [ ] DTO 结构体字段完整（含 JSON tag）
  - [ ] Registry 可注册和获取 Provider 工厂
  - [ ] 编译通过，无循环依赖

  **QA Scenarios**:

  ```
  Scenario: Provider 注册表功能
    Tool: Bash (go test)
    Preconditions: 无
    Steps:
      1. 编写 mock provider 注册到 registry
      2. 通过 registry.Get("mock") 获取
      3. 验证返回的 provider 正确
    Expected Result: 注册表正确注册和获取
    Failure Indicators: 获取失败、类型不匹配
    Evidence: .sisyphus/evidence/task-5-registry.txt

  Scenario: DTO JSON 序列化
    Tool: Bash (go test)
    Preconditions: 无
    Steps:
      1. 构造 CreatePaymentResponse
      2. json.Marshal 序列化
      3. 验证 JSON 字段名与 EasyPay 协议一致（如 trade_no、payurl 等）
    Expected Result: JSON 字段名正确
    Failure Indicators: 字段名大小写错误、缺失字段
    Evidence: .sisyphus/evidence/task-5-dto-json.txt
  ```

  **Commit**: YES
  - Message: `feat: provider interface, types, and registry`
  - Files: `internal/provider/types.go`, `internal/provider/registry.go`

- [x] 6. EasyPay 协议处理器 — Handler 层（验签 + 路由 + 回调转发）

  **What to do**:
  - 实现 `POST /mapi.php`：解析 form 参数 → 验签 → 创建订单 → 调 Provider 获取支付链接/二维码 → 返回 JSON（code/message/trade_no/payurl/qrcode）
  - 实现 `GET /submit.php`：解析 query 参数 → 验签 → 创建订单 → 302 重定向到支付页面
  - 实现 `POST /api.php?act=order`：解析参数 → 验签（pid+key 模式）→ 查询订单状态 → 返回 JSON
  - 实现签名验证中间件/函数：按 key 排序 → 拼接 → append pkey → MD5 → 比对
  - 实现 `out_trade_no` 重复检测：同一 out_trade_no + 不同参数 → 拒绝
  - 实现 `notify_url` 校验：禁止包含 `?` 查询参数，禁止内网 IP（127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16）
  - 实现错误响应格式：`{"code": -1/-3, "msg": "..."}`（-1 通用失败，-3 签名错误）

  **Must NOT do**:
  - 不要在这里实现支付宝/微信的具体调用（那是 Provider 的事）
  - 不要在这里处理费率计算（那是 Service 的事）
  - 不要在 handler 层直接操作数据库

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 协议层需要精确复现 EasyPay 行为，参考多个开源实现
  - **Skills**: [`go`, `gin`, `security`]

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 5, 7, 8)
  - **Blocks**: Task 9 (支付 Service)
  - **Blocked By**: Task 2 (Order 表), Task 5 (DTO 类型)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\provider\easypay.go` — EasyPay 签名算法、参数处理、响应格式的完整客户端实现，可反推服务端
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\provider\easypay_sign_test.go` — 签名算法的测试用例（输入输出验证）
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\handler\payment_webhook_handler.go` — 回调处理器模式
  - Open-source EasyPay PHP 实现（Blokura/Epay, v03413/Epay）：`mapi.php` 的重复订单检测逻辑

  **Acceptance Criteria**:
  - [ ] `POST /mapi.php` 正确验签并返回支付链接
  - [ ] 错误签名返回 `{"code": -3, "msg": "签名错误"}`
  - [ ] 相同 `out_trade_no` 不同金额的重复调用被拒绝
  - [ ] `notify_url` 包含 `?` 的请求被拒绝
  - [ ] `notify_url` 指向 `127.0.0.1` 的请求被拒绝

  **QA Scenarios**:

  ```
  Scenario: 正常创建支付（验签通过）
    Tool: Bash (curl)
    Preconditions: 测试商户(pid=1001, pkey=testkey)已存在
    Steps:
      1. 构造签名正确的 form 参数
      2. curl -X POST http://localhost:8080/mapi.php -d "pid=1001&type=alipay&out_trade_no=QA001&name=test&money=0.01&notify_url=http://example.com/notify&sign=<计算>&sign_type=MD5"
    Expected Result: {"code":1,"trade_no":"...","qrcode":"..."} 或 {"code":1,"payurl":"..."}
    Failure Indicators: code != 1, 签名错误, 500 错误
    Evidence: .sisyphus/evidence/task-6-mapi-create.txt

  Scenario: 签名错误被拒绝
    Tool: Bash (curl)
    Preconditions: 同上
    Steps:
      1. 使用错误 sign 调用 /mapi.php
    Expected Result: {"code":-3,"msg":"签名错误"}
    Failure Indicators: code=1 接受了错误签名
    Evidence: .sisyphus/evidence/task-6-sign-reject.txt

  Scenario: notify_url 包含查询参数被拒绝
    Tool: Bash (curl)
    Preconditions: 同上
    Steps:
      1. notify_url=http://example.com/notify?foo=bar
    Expected Result: {"code":-1,"msg":"notify_url不能包含查询参数"}
    Failure Indicators: 请求被接受
    Evidence: .sisyphus/evidence/task-6-notify-url-reject.txt

  Scenario: 重复订单被拒绝
    Tool: Bash (curl)
    Preconditions: out_trade_no=QA001 已创建
    Steps:
      1. 用相同 out_trade_no 但不同金额再次调用
    Expected Result: {"code":-1,"msg":"订单已存在且参数不一致"}
    Failure Indicators: 新订单创建成功
    Evidence: .sisyphus/evidence/task-6-duplicate-reject.txt
  ```

  **Commit**: YES
  - Message: `feat: easypay protocol handler with sign verification and validation`
  - Files: `internal/handler/easypay/*.go`, `internal/handler/middleware/sign.go`

- [x] 7. 支付宝 Provider — 扫码/跳转支付实现

  **What to do**:
  - 安装 `github.com/smartwalle/alipay/v3` SDK
  - 实现 `AlipayProvider` 结构体，满足 `Provider` 接口
  - 构造函数接收 `appId`、`privateKey`、`alipayPublicKey`、`notifyUrl`、`returnUrl`
  - `CreatePayment` 实现：
    - 桌面端：优先 `TradePreCreate`（当面付返回二维码串）；失败则回退 `TradePagePay`（电脑网站支付）
    - 移动端：`TradeWapPay`（手机网站支付）
  - `QueryOrder`：调用 `TradeQuery`
  - `VerifyNotification`：调用 `DecodeNotification` 验签，解析交易状态
  - 错误处理：`ACQ.TRADE_NOT_EXIST` 特殊处理为 pending 状态
  - 回调响应写回 `"success"` 纯文本

  **Must NOT do**:
  - 不要硬编码支付宝配置（从 config 读）
  - 不要实现退款功能
  - 不要在 Provider 中操作数据库

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 需要精确对接支付宝 SDK，处理多种支付场景和错误码
  - **Skills**: [`go`, `payment`, `security`]

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 8 完全独立）
  - **Parallel Group**: Wave 2 (with Tasks 5, 6, 8)
  - **Blocks**: Task 9 (支付 Service 需要实际的 Provider 实例)
  - **Blocked By**: Task 5 (Provider 接口), Task 4 (配置)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\provider\alipay.go` — **MUST READ**：完整的支付宝 Provider 实现，包含桌面/Mobile 路由、Precreate 优先策略、错误码处理
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\provider\alipay_test.go` — 支付宝 Provider 测试模式
  - `https://github.com/smartwalle/alipay` — 支付宝 SDK 文档

  **Acceptance Criteria**:
  - [ ] `CreatePayment`（桌面）返回有效二维码串
  - [ ] `CreatePayment`（移动）返回支付宝跳转链接
  - [ ] `VerifyNotification` 正确验签支付宝回调
  - [ ] `QueryOrder` 可查询支付状态
  - [ ] SDK 变量注入支持测试（参考 sub2api 的 `var alipayTradePreCreate` 模式）

  **QA Scenarios**:

  ```
  Scenario: 创建桌面扫码支付
    Tool: Bash (go test + mock)
    Preconditions: 支付宝 sandbox 或 mock 配置
    Steps:
      1. 调用 CreatePayment(ctx, CreatePaymentRequest{Amount: "0.01", Subject: "测试", IsMobile: false})
      2. 验证返回 QRCode 非空
    Expected Result: QRCode 为有效支付宝二维码串（https://qr.alipay.com/...）
    Failure Indicators: QRCode 为空、返回错误
    Evidence: .sisyphus/evidence/task-7-alipay-create.txt

  Scenario: 验证支付宝回调
    Tool: Bash (go test)
    Preconditions: 构造合法的支付宝回调通知 body
    Steps:
      1. 使用支付宝 SDK 签名测试数据
      2. 调用 VerifyNotification
      3. 验证返回 status=success 且 amount 正确
    Expected Result: 验签通过，状态正确解析
    Failure Indicators: 验签失败、金额解析错误
    Evidence: .sisyphus/evidence/task-7-alipay-verify.txt
  ```

  **Commit**: YES
  - Message: `feat: alipay provider implementation`
  - Files: `internal/provider/alipay/*.go`

- [x] 8. 微信支付 Provider — Native/H5 支付实现

  **What to do**:
  - 安装微信支付 APIv3 Go SDK 或自实现 HTTP 签名
  - 实现 `WxpayProvider` 结构体，满足 `Provider` 接口
  - 构造函数接收 `appId`、`mchId`、`privateKey`(PEM)、`apiV3Key`、`publicKey`(PEM)、`publicKeyId`、`serialNo`、`notifyUrl`
  - `CreatePayment` 实现：
    - 桌面端：`Native` 支付 → 返回 code_url（二维码串）
    - 移动端：`H5` 支付 → 返回 h5_url（跳转链接）
  - `QueryOrder`：调用微信订单查询 API
  - `VerifyNotification`：
    - 验证 HTTP 头中的签名（Timestamp + Nonce + Signature）
    - 解密回调 body（AES-256-GCM）
    - 解析交易状态
  - 回调响应：返回 HTTP 200 + JSON `{"code": "SUCCESS", "message": "成功"}`

  **Must NOT do**:
  - 不要实现 JSAPI/公众号支付（首版不需要）
  - 不要硬编码微信配置
  - 不要实现退款功能

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 微信支付 APIv3 签名和回调解密较复杂
  - **Skills**: [`go`, `payment`, `security`]

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 7 完全独立）
  - **Parallel Group**: Wave 2 (with Tasks 5, 6, 7)
  - **Blocks**: Task 9 (支付 Service)
  - **Blocked By**: Task 5 (Provider 接口), Task 4 (配置)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\provider\wxpay.go` — **MUST READ**：微信支付 Provider 完整实现，含签名和回调解密
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\provider\wxpay_test.go` — 微信 Provider 测试模式
  - `https://pay.weixin.qq.com/docs/merchant/development/native-pay/prepay.html` — 微信 Native 支付文档

  **Acceptance Criteria**:
  - [ ] `CreatePayment`（桌面）返回有效 code_url
  - [ ] `CreatePayment`（移动）返回 h5_url
  - [ ] `VerifyNotification` 正确验签和解密微信回调
  - [ ] `QueryOrder` 可查询支付状态

  **QA Scenarios**:

  ```
  Scenario: 创建 Native 扫码支付
    Tool: Bash (go test + mock)
    Preconditions: 微信 sandbox 或 mock 配置
    Steps:
      1. 调用 CreatePayment(ctx, CreatePaymentRequest{Amount: "0.01", Subject: "测试", IsMobile: false})
      2. 验证返回 QRCode 非空（weixin://wxpay/bizpayurl/...）
    Expected Result: QRCode 为有效微信支付链接
    Failure Indicators: QRCode 为空、返回错误
    Evidence: .sisyphus/evidence/task-8-wxpay-create.txt

  Scenario: 验证微信回调
    Tool: Bash (go test)
    Preconditions: 构造合法的微信回调（含签名头）
    Steps:
      1. 构造加密的微信回调 body
      2. 设置正确的 HTTP 头（Wechatpay-Timestamp, Nonce, Signature）
      3. 调用 VerifyNotification
      4. 验证返回 status=success
    Expected Result: 签名验证通过，解密成功，状态正确
    Failure Indicators: 签名验证失败、解密失败、状态错误
    Evidence: .sisyphus/evidence/task-8-wxpay-verify.txt
  ```

  **Commit**: YES
  - Message: `feat: wechat pay provider implementation`
  - Files: `internal/provider/wxpay/*.go`

- [x] 9. 支付 Service — 订单创建/查询/回调处理

  **What to do**:
  - 实现 `PaymentService` 核心结构体
  - `CreateOrder` 方法：
    - 校验参数（金额范围、notify_url）
    - 从 registry 获取对应 Provider 实例
    - 保存 Provider 配置快照到订单（JSONB `provider_snapshot`）
    - 调用 Provider.CreatePayment
    - 保存订单到数据库（状态 PENDING）
    - 返回支付信息给 handler
  - `HandleCallback` 方法：
    - Redis 幂等检查（out_trade_no）
    - 调 Provider.VerifyNotification 验签
    - 从订单快照交叉验证 Provider 身份一致性
    - 金额容差校验：`|paid - expected| ≤ 0.01`
    - 更新订单状态 PAID
    - 异步转发回调通知到商户 notify_url（重试机制：10s→60s→10min→30min→60min）
  - `QueryOrder` 方法：
    - 从 DB 查询 → 若 PENDING 则调 Provider.QueryOrder 同步状态
  - 后台定时任务：扫描超时 PENDING 订单 → 先查上游状态 → 标记 EXPIRED

  **Must NOT do**:
  - 不要在 handler 层写业务逻辑
  - 不要同步等待回调转发（异步 goroutine）
  - 不在首版实现退款

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: 核心支付流程，涉及多个子系统协调，容错要求高
  - **Skills**: [`go`, `database`, `concurrency`]

  **Parallelization**:
  - **Can Run In Parallel**: NO（依赖 Tasks 5-8 完成）
  - **Parallel Group**: Wave 3 (with Tasks 10, 11, 12)
  - **Blocks**: Tasks 16, 17 (Admin/Merchant API)
  - **Blocked By**: Tasks 2 (Order schema), 3 (Redis), 6 (EasyPay handler 结构), 7, 8 (Provider 实现)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\service\payment_order_lifecycle.go` — 订单生命周期管理
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\service\payment_fulfillment.go` — 回调处理和余额充值
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\service\payment_order_provider_snapshot.go` — **CRITICAL**：Provider 快照保存模式

  **Acceptance Criteria**:
  - [ ] `CreateOrder` 成功创建支付宝和微信订单
  - [ ] `HandleCallback` 幂等处理（重复回调不重复入账）
  - [ ] 回调后订单状态正确更新为 PAID
  - [ ] 超时订单被后台任务标记为 EXPIRED
  - [ ] 回调转发到商户 notify_url 成功

  **QA Scenarios**:

  ```
  Scenario: 完整支付流程（支付宝）
    Tool: Bash (curl + go test)
    Preconditions: 商户 test_merchant 已创建
    Steps:
      1. CreateOrder(alipay, 0.01) → 返回 qrcode
      2. 构造支付宝回调通知
      3. HandleCallback → 验证订单状态变 PAID
    Expected Result: 订单从 PENDING → PAID
    Failure Indicators: 创建失败、回调处理失败、状态未更新
    Evidence: .sisyphus/evidence/task-9-payment-flow.txt

  Scenario: 回调幂等
    Tool: Bash (curl)
    Preconditions: 订单已 PAID
    Steps:
      1. 用相同参数再次调用回调
      2. 验证商户余额未重复增加
    Expected Result: 第二次回调被忽略（幂等），余额不变
    Failure Indicators: 余额被重复增加
    Evidence: .sisyphus/evidence/task-9-idempotent-callback.txt
  ```

  **Commit**: YES
  - Message: `feat: payment service with order lifecycle and callback handling`
  - Files: `internal/service/payment/*.go`

- [x] 10. 费率 Service — 费率计算 + 余额入账

  **What to do**:
  - 实现 `FeeService`
  - `CalculateFee` 方法：
    - 读取全局配置：`official_alipay_rate` `official_wxpay_rate`、`default_platform_rate`
    - 读取商户自定义费率（如存在则覆盖全局）
    - 计算：`fee_official = amount * official_rate`，`fee_platform = amount * platform_rate`，`net_amount = amount - fee_platform`
  - `ApplySettlement` 方法（回调处理时调用）：
    - 调用 `CalculateFee`
    - 更新订单的 fee 字段和 status → SETTLED
    - 更新商户 Settlement 表：`balance += net_amount`，`total_income += amount`
    - 使用 Redis 分布式锁防止并发结算
  - `GetMerchantBalance` 方法：查询 settlement 表返回余额

  **Must NOT do**:
  - 不要在此 service 中处理提现（交给 Task 11）
  - 费率计算不要硬编码，全部从 configs 表读取

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 费率计算逻辑不复杂但需精确，涉及金额精度处理
  - **Skills**: [`go`, `database`]

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 11 独立，依赖 Task 9）
  - **Parallel Group**: Wave 3 (with Tasks 9, 11, 12)
  - **Blocks**: Task 16 (Admin API 需要费率查询)
  - **Blocked By**: Task 2 (Settlement/Config schema), Task 3 (Redis 锁)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\fee.go` — 费率计算参考
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\service\payment_amounts.go` — 金额处理
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\payment\amount.go` — 金额格式化

  **Acceptance Criteria**:
  - [ ] 官方费率 0.6%、平台费率 1% 时，100 元订单：fee_official=0.6, fee_platform=1.0, net_amount=99.0
  - [ ] 商户自定义费率覆盖全局默认
  - [ ] 余额计算正确（执行多次结算后累加正确）
  - [ ] 并发结算时分布式锁防止余额错误

  **QA Scenarios**:

  ```
  Scenario: 费率计算正确
    Tool: Bash (go test)
    Preconditions: 全局配置 official_alipay_rate=0.6%, default_platform_rate=1%
    Steps:
      1. CalculateFee(alipay, 100.00, merchant_id)
      2. 验证 fee_official=0.6, fee_platform=1.0, net_amount=99.0
    Expected Result: 三项金额计算精确
    Failure Indicators: 金额精度错误（如 99.0000000001）
    Evidence: .sisyphus/evidence/task-10-fee-calc.txt

  Scenario: 结算后余额正确
    Tool: Bash (curl)
    Preconditions: 订单已 PAID
    Steps:
      1. 触发结算
      2. 查询商户 balance
    Expected Result: balance = 原有余额 + net_amount
    Failure Indicators: 余额未变、金额错误
    Evidence: .sisyphus/evidence/task-10-settlement.txt
  ```

  **Commit**: YES
  - Message: `feat: fee calculation and settlement service`
  - Files: `internal/service/fee/*.go`

- [x] 11. 结算 Service — 余额管理 + 提现申请/审核

  **What to do**:
  - 实现 `SettlementService`
  - `RequestWithdraw` 方法：
    - 校验金额 ≤ balance
    - 冻结金额：`balance -= amount`, `frozen += amount`
    - 创建 Withdraw 记录（status=PENDING）
  - `ApproveWithdraw` 方法（管理员调用）：
    - 更新状态 APPROVED
  - `ConfirmWithdraw` 方法（管理员手动打款后确认）：
    - 更新状态 PAID
    - `frozen -= amount`, `total_withdrawn += amount`
  - `RejectWithdraw` 方法：
    - 更新状态 REJECTED
    - 退回冻结金额：`frozen -= amount`, `balance += amount`
  - `GetWithdrawList`：分页查询，支持按 merchant_id/status 筛选
  - 并发用 Redis 分布式锁保护

  **Must NOT do**:
  - 不要自动打款
  - 不要实现批量提现（一次一条）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: 关键资金操作，需要事务和锁保证正确性
  - **Skills**: [`go`, `database`, `concurrency`]

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 10 独立）
  - **Parallel Group**: Wave 3 (with Tasks 9, 10, 12)
  - **Blocks**: Task 16, 17 (API 需要提现查询)
  - **Blocked By**: Task 2 (Settlement/Withdraw schema), Task 3 (Redis 锁)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\service\payment_refund.go` — 退款流程的事务处理模式参考
  - Ent 事务文档：`https://entgo.io/docs/transactions`

  **Acceptance Criteria**:
  - [ ] 提现申请 → 余额减少、frozen 增加
  - [ ] 审核通过 → 状态 APPROVED
  - [ ] 确认打款 → 状态 PAID + frozen 清零
  - [ ] 拒绝提现 → 退回余额 + 状态 REJECTED
  - [ ] 余额不足时提现被拒绝

  **QA Scenarios**:

  ```
  Scenario: 提现申请和审核流程
    Tool: Bash (curl)
    Preconditions: 商户余额 ≥ 50
    Steps:
      1. RequestWithdraw(50) → 余额-50, frozen+50, 提现状态 PENDING
      2. ApproveWithdraw → 状态 APPROVED
      3. ConfirmWithdraw → 状态 PAID, frozen-50
    Expected Result: 每一步状态和余额变化正确
    Failure Indicators: 金额不对、状态不对
    Evidence: .sisyphus/evidence/task-11-withdraw-flow.txt

  Scenario: 提现拒绝退回余额
    Tool: Bash (curl)
    Preconditions: 商户已申请提现 30
    Steps:
      1. RejectWithdraw → 状态 REJECTED
      2. 查余额 → 余额恢复了 30
    Expected Result: 余额退回
    Failure Indicators: 余额未恢复
    Evidence: .sisyphus/evidence/task-11-withdraw-reject.txt
  ```

  **Commit**: YES
  - Message: `feat: settlement service with withdrawal management`
  - Files: `internal/service/settlement/*.go`

- [x] 12. 商户 Service + Admin Service — 商户管理

  **What to do**:
  - 实现 `MerchantService`
  - `CreateMerchant`：生成唯一 pid（自增或随机），生成 32 位随机 pkey，默认平台费率
  - `GetMerchant`、`UpdateMerchant`（更新费率/状态/名称）
  - `ListMerchants`：分页 + 筛选
  - `RegeneratePkey`：重新生成密钥（旧密钥立即失效）
  - 实现 `AdminService`：
    - `Login`：用户名密码验证，返回 JWT token
    - `ChangePassword`
  - 实现管理员认证中间件（JWT 验证）

  **Must NOT do**:
  - 不要实现多管理员/RBAC
  - 不要在 API 响应中暴露 pkey（仅在商户端 API 密钥页面返回完整值）

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: CRUD + 认证，常规但涉及安全
  - **Skills**: [`go`, `database`, `security`]

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 3 (with Tasks 9, 10, 11)
  - **Blocks**: Tasks 16 (Admin API), 17 (Merchant API)
  - **Blocked By**: Task 2 (Merchant/Admin schema)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\service\user_service.go` — 用户服务模式
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\handler\auth_handler.go` — JWT 认证处理

  **Acceptance Criteria**:
  - [ ] 创建商户自动生成 pid/pkey
  - [ ] JWT 登录成功返回 token
  - [ ] 无 token 访问管理 API 返回 401
  - [ ] 管理员更新商户费率成功

  **QA Scenarios**:

  ```
  Scenario: 创建商户
    Tool: Bash (curl)
    Preconditions: 管理员已登录（token）
    Steps:
      1. POST /api/admin/merchants -d '{"name":"测试商户","fee_rate":1.5}'
      2. 验证返回 pid/pkey
    Expected Result: 201 + 返回商户信息含 pid/pkey
    Failure Indicators: pkey 未生成、pid 重复
    Evidence: .sisyphus/evidence/task-12-create-merchant.txt

  Scenario: 管理员登录
    Tool: Bash (curl)
    Preconditions: 管理员账号已创建
    Steps:
      1. POST /api/admin/login -d '{"username":"admin","password":"admin123"}'
      2. 验证返回 JWT token
    Expected Result: 200 + token，后续请求可用
    Failure Indicators: 认证失败、token 无效
    Evidence: .sisyphus/evidence/task-12-admin-login.txt
  ```

  **Commit**: YES
  - Message: `feat: merchant service and admin authentication`
  - Files: `internal/service/merchant/*.go`, `internal/service/admin/*.go`

- [x] 13. Vue3 项目搭建 — Vite + NaiveUI + Tailwind + 路由

  **What to do**:
  - 用 `pnpm create vite` 创建 Vue3 + TypeScript 项目
  - 安装依赖：`naive-ui`、`vue-router`、`pinia`、`axios`、`tailwindcss`、`@tailwindcss/vite`
  - 搭建目录结构：`src/{views,components,api,stores,router,composables}`
  - 配置 Naive UI 全局主题（暗黑模式支持，主题色）
  - 配置 Vue Router：admin 路由（`/admin/*`）和 merchant 路由（`/merchant/*`）
  - 配置 Axios 拦截器（baseURL、token 注入、401 跳转登录）
  - 创建基础布局组件：侧边栏 + 顶栏 + 内容区
  - 配置 Tailwind v4（`@import "tailwindcss"`）

  **Must NOT do**:
  - 不要引入 Element Plus 或其他组件库（只用 Naive UI）
  - 不要实现任何业务页面（只是骨架）

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: 前端搭建 + UI 框架配置
  - **Skills**: [`vue`, `tailwind`, `typescript`]

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Wave 1-3 后端任务完全独立）
  - **Parallel Group**: Wave 4 (with Tasks 14, 15)
  - **Blocks**: Tasks 14, 15 (页面实现)
  - **Blocked By**: None（纯前端，不依赖后端）

  **Acceptance Criteria**:
  - [ ] `pnpm run dev` 启动成功
  - [ ] 访问 `http://localhost:5173/admin` 显示空白布局（侧边栏+内容区）
  - [ ] 访问 `http://localhost:5173/merchant` 显示空白布局
  - [ ] 暗黑模式切换按钮可用
  - [ ] `pnpm run build` 构建成功（无错误）

  **QA Scenarios**:

  ```
  Scenario: 项目启动和路由
    Tool: Playwright
    Preconditions: pnpm install 完成
    Steps:
      1. 导航到 http://localhost:5173/admin
      2. 验证侧边栏和内容区渲染
      3. 导航到 /merchant
      4. 验证布局切换
    Expected Result: 两个布局正常渲染，路由切换无错误
    Failure Indicators: 白屏、404、控制台报错
    Evidence: .sisyphus/evidence/task-13-scaffold.png + .sisyphus/evidence/task-13-scaffold.txt
  ```

  **Commit**: YES
  - Message: `feat: vue3 project setup with naive-ui, tailwind, router`
  - Files: `frontend/*`（package.json, vite.config.ts, src/）

- [x] 14. 管理后台页面 — 登录/商户管理/订单/提现审核/系统配置

  **What to do**:
  基于 Naive UI 组件实现以下页面（所有页面需要暗黑模式兼容）：
  - **登录页**：用户名+密码表单，JWT 认证
  - **仪表盘**：今日订单数、今日收入、待审核提现数（卡片 + 简单图表）
  - **商户管理**：表格（分页） + 新建/编辑弹窗（名称、费率、状态开关），操作列：查看密钥、重新生成
  - **订单列表**：表格（分页，按状态/商户/时间筛选），状态标签用颜色区分，详情弹窗
  - **提现审核**：表格（筛选 PENDING/APPROVED），操作列：通过/拒绝，确认弹窗
  - **系统配置**：表单（支付宝 AppID/密钥、微信商户号/密钥、默认费率），API 密钥用密码框
  - 布局：侧边栏导航 + 面包屑 + 用户信息/退出

  **Must NOT do**:
  - 不要用 Element Plus 或 Ant Design
  - 密钥类字段在表格中显示为 `***`（脱敏）

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: 完整的后台页面开发，涉及大量 Naive UI 组件使用
  - **Skills**: [`vue`, `naive-ui`, `tailwind`, `typescript`]

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 15 独立）
  - **Parallel Group**: Wave 4 (with Tasks 13, 15)
  - **Blocks**: Task 19 (集成测试)
  - **Blocked By**: Task 13 (前端搭建)

  **References**:
  - `https://www.naiveui.com/zh-CN/os-theme/components/data-table` — Naive UI 表格组件
  - `https://www.naiveui.com/zh-CN/os-theme/components/form` — Naive UI 表单组件
  - `C:\Users\zhengjiyong\code\sub2api\frontend\src\views\` — sub2api 管理后台页面结构参考

  **Acceptance Criteria**:
  - [ ] 所有 6 个页面可正常渲染
  - [ ] 商户 CRUD 完整可用
  - [ ] 提现审核流程完整可用
  - [ ] 系统配置可保存
  - [ ] 暗黑模式所有页面正常

  **QA Scenarios**:

  ```
  Scenario: 登录并创建商户
    Tool: Playwright
    Preconditions: 后端运行中
    Steps:
      1. 导航到 /admin/login
      2. 填写用户名密码，点击登录
      3. 验证跳转到仪表盘
      4. 导航到商户管理
      5. 点击"新建商户"，填写名称、费率，保存
      6. 验证列表中新增一条记录
    Expected Result: 完整流程无报错
    Failure Indicators: 接口 401/500、创建失败
    Evidence: .sisyphus/evidence/task-14-admin-flow.png

  Scenario: 审核提现
    Tool: Playwright
    Preconditions: 有 PENDING 状态的提现申请
    Steps:
      1. 导航到提现审核
      2. 点击某条的"通过"按钮
      3. 确认弹窗点击确定
      4. 验证状态变为 APPROVED
    Expected Result: 状态更新成功
    Failure Indicators: 状态未变、弹窗无响应
    Evidence: .sisyphus/evidence/task-14-withdraw-review.png
  ```

  **Commit**: YES
  - Message: `feat: admin dashboard pages with merchant/order/withdraw management`
  - Files: `frontend/src/views/admin/`, `frontend/src/api/admin.ts`

- [x] 15. 商户端页面 — 注册/登录/订单/余额/提现/API密钥

  **What to do**:
  基于 Naive UI 组件实现商户端页面（比管理后台简单，不需要复杂权限）：
  - **注册页**：商户名 + 密码注册（或简化为管理员创建）
  - **登录页**：pid + 密码登录
  - **首页**：余额卡片、今日订单数、累计收入
  - **订单列表**：自己的订单（表格 + 分页 + 状态筛选），显示金额、手续费、净额
  - **提现申请**：表单（金额 + 收款账号），当前余额显示
  - **提现记录**：历史提现列表，状态标签
  - **API 密钥**：显示 pid/pkey，复制按钮，回调地址设置
  - 布局：侧边栏导航 + 顶栏用户信息

  **Must NOT do**:
  - 不要暴露其他商户的数据
  - 不要在商户端提供费率修改功能

  **Recommended Agent Profile**:
  - **Category**: `visual-engineering`
    - Reason: 商户端页面，与 Task 14 类似但更轻量
  - **Skills**: [`vue`, `naive-ui`, `tailwind`, `typescript`]

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 14 独立）
  - **Parallel Group**: Wave 4 (with Tasks 13, 14)
  - **Blocks**: Task 19 (集成测试)
  - **Blocked By**: Task 13 (前端搭建)

  **References**:
  - `https://www.naiveui.com/zh-CN/os-theme/components/input` — Naive UI 输入组件（密钥复制用）
  - `https://www.naiveui.com/zh-CN/os-theme/components/statistic` — Naive UI 统计数值组件

  **Acceptance Criteria**:
  - [ ] 注册/登录流程完整
  - [ ] 订单列表只显示自己的订单
  - [ ] 提现申请表单验证（金额 > 0 且 ≤ 余额）
  - [ ] API 密钥页面可复制 pid/pkey

  **QA Scenarios**:

  ```
  Scenario: 商户登录查看订单和提现
    Tool: Playwright
    Preconditions: 商户账号已存在，有订单数据
    Steps:
      1. 导航到 /merchant/login
      2. 输入 pid 和密码登录
      3. 验证首页显示余额
      4. 导航到订单列表，验证有订单数据
      5. 导航到提现申请，输入金额提交
      6. 验证提交成功提示
    Expected Result: 完整商户流程无报错
    Failure Indicators: 余额显示错误、订单数据为空、提现提交失败
    Evidence: .sisyphus/evidence/task-15-merchant-flow.png

  Scenario: 查看和复制 API 密钥
    Tool: Playwright
    Preconditions: 商户已登录
    Steps:
      1. 导航到 API 密钥页面
      2. 验证 pid 和 pkey 显示
      3. 点击 pkey 旁的复制按钮
      4. 验证剪贴板内容 = pkey
    Expected Result: 密钥正确显示和复制
    Failure Indicators: pkey 未显示、复制失败
    Evidence: .sisyphus/evidence/task-15-api-key.png
  ```

  **Commit**: YES
  - Message: `feat: merchant portal pages with order/withdraw/key management`
  - Files: `frontend/src/views/merchant/`, `frontend/src/api/merchant.ts`

- [x] 16. Admin API — 管理后台后端接口

  **What to do**:
  - 实现 `/api/admin/*` 路由组（JWT 中间件保护）
  - `POST /api/admin/login` — 管理员登录
  - `GET/POST /api/admin/merchants` — 商户 CRUD
  - `PUT /api/admin/merchants/:id` — 更新商户（费率/状态）
  - `POST /api/admin/merchants/:id/regenerate-key` — 重新生成 pkey
  - `GET /api/admin/orders` — 全平台订单（分页/筛选）
  - `GET /api/admin/dashboard` — 仪表盘数据
  - `GET /api/admin/withdraws` — 提现列表（按状态筛选）
  - `POST /api/admin/withdraws/:id/approve` / `reject` / `confirm`
  - `GET/PUT /api/admin/configs` — 系统配置读写
  - 响应格式统一：`{"code": 0, "data": {...}, "message": "ok"}`

  **Must NOT do**:
  - 不要在 admin API 中暴露 pkey 明文（列表返回脱敏值）
  - 不要跳过 JWT 验证

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: [`go`, `gin`, `jwt`]

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 17 独立）
  - **Parallel Group**: Wave 5 (with Tasks 17, 18, 19, 20)
  - **Blocks**: Task 19 (集成测试)
  - **Blocked By**: Tasks 9-12 (所有 Service)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\handler\admin\` — Admin handler 模式参考

  **Acceptance Criteria**:
  - [ ] 所有 admin 接口返回正确数据
  - [ ] 无 token 请求返回 401

  **QA Scenarios**:

  ```
  Scenario: 管理员登录 + 商户 CRUD
    Tool: Bash (curl)
    Preconditions: 管理员账号存在
    Steps:
      1. POST /api/admin/login → 获取 token
      2. POST /api/admin/merchants → 创建商户
      3. GET /api/admin/merchants → 列表含新商户
      4. PUT /api/admin/merchants/:id → 更新费率
    Expected Result: CRUD 全部 200
    Failure Indicators: 认证失败、创建失败
    Evidence: .sisyphus/evidence/task-16-admin-api.txt
  ```

  **Commit**: YES
  - Message: `feat: admin API endpoints with JWT auth`
  - Files: `internal/handler/admin/*.go`

- [x] 17. Merchant API — 商户端后端接口

  **What to do**:
  - 实现 `/api/merchant/*` 路由组（商户 JWT 中间件保护）
  - `POST /api/merchant/login` / `register` — 商户登录/注册
  - `GET /api/merchant/profile` — 获取当前商户信息
  - `GET /api/merchant/orders` — 自己的订单（分页/状态筛选）
  - `GET /api/merchant/balance` — 余额查询
  - `POST /api/merchant/withdraws` — 提现申请
  - `GET /api/merchant/withdraws` — 提现记录
  - `GET /api/merchant/api-key` — 查看 pid/pkey（完整值）
  - `PUT /api/merchant/notify-url` — 设置默认回调地址

  **Must NOT do**:
  - 不要在 merchant API 中返回其他商户数据

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
  - **Skills**: [`go`, `gin`, `jwt`]

  **Parallelization**:
  - **Can Run In Parallel**: YES（与 Task 16 独立）
  - **Parallel Group**: Wave 5 (with Tasks 16, 18, 19, 20)
  - **Blocks**: Task 19 (集成测试)
  - **Blocked By**: Tasks 9-12 (所有 Service)

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\backend\internal\handler\user_handler.go` — 用户 handler 模式

  **Acceptance Criteria**:
  - [ ] 订单列表只有自己的订单
  - [ ] 提现申请成功扣除余额

  **QA Scenarios**:

  ```
  Scenario: 商户登录并查看自己的订单
    Tool: Bash (curl)
    Preconditions: 商户 test_merchant (pid=1001) 存在且有订单
    Steps:
      1. POST /api/merchant/login → 获取 token
      2. GET /api/merchant/orders → 验证只返回自己的订单
    Expected Result: 订单数据隔离正确
    Failure Indicators: 返回了其他商户订单
    Evidence: .sisyphus/evidence/task-17-merchant-api.txt
  ```

  **Commit**: YES
  - Message: `feat: merchant API endpoints`
  - Files: `internal/handler/merchant/*.go`

- [x] 18. Docker Compose — 完整部署配置

  **What to do**:
  - 完善 `docker-compose.yml`：postgres + redis + epay 三容器
  - 创建 `.env.example`：数据库密码、JWT Secret、初始管理员
  - Dockerfile 多阶段构建：Go 编译 → Node 编译前端 → Alpine 运行镜像
  - 健康检查配置
  - `docker compose up -d` 一键启动

  **Must NOT do**:
  - 不要在镜像中硬编码密钥

  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: [`docker`, `devops`]

  **Parallelization**:
  - **Can Run In Parallel**: NO（依赖所有后端+前端完成）
  - **Parallel Group**: Wave 5
  - **Blocks**: Task 19 (集成测试)
  - **Blocked By**: Tasks 1, 14, 15

  **References**:
  - `C:\Users\zhengjiyong\code\sub2api\deploy\docker-compose.yml` — Docker Compose 模板
  - `C:\Users\zhengjiyong\code\sub2api\Dockerfile` — 多阶段构建参考

  **Acceptance Criteria**:
  - [ ] `docker compose up -d` 三容器全部启动 healthy
  - [ ] `curl http://localhost:8080/health` 返回 200

  **QA Scenarios**:

  ```
  Scenario: 一键启动全栈
    Tool: Bash
    Preconditions: Docker Desktop 运行
    Steps:
      1. docker compose up -d --build
      2. docker compose ps → 三服务 healthy
      3. curl http://localhost:8080/health
    Expected Result: 全部 healthy
    Failure Indicators: 容器退出、健康检查失败
    Evidence: .sisyphus/evidence/task-18-docker-up.txt
  ```

  **Commit**: YES
  - Message: `feat: docker compose deployment`
  - Files: `docker-compose.yml`, `.env.example`, `Dockerfile`

- [x] 19. 集成测试 — 端到端支付流程验证

  **What to do**:
  - 启动 `docker compose up -d`
  - 管理员登录 → 创建商户 → 配置支付宝/微信 sandbox
  - 商户调用 `/mapi.php` 创建支付
  - 模拟支付宝/微信回调 → 验证订单 PAID → 结算
  - 商户申请提现 → 管理员审核 → 确认打款
  - 测试错误场景：错误签名、重复订单、SSRF
  - Playwright 验证管理后台和商户端页面
  - 证据保存到 `.sisyphus/evidence/final-qa/`

  **Must NOT do**:
  - 不要写 Go 测试（curl + Playwright 直接验证）

  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: [`playwright`, `curl`]

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 5
  - **Blocks**: F3 (Real Manual QA)
  - **Blocked By**: Tasks 16, 17, 18

  **Acceptance Criteria**:
  - [ ] 完整支付→结算→提现流程通过
  - [ ] 所有错误场景正确拒绝

  **QA Scenarios**:

  ```
  Scenario: 完整端到端支付流程
    Tool: Bash (curl) + Playwright
    Preconditions: docker compose 全栈启动
    Steps:
      1. 管理员创建商户
      2. 商户调用 /mapi.php 创建支付
      3. 模拟回调 → 验证余额增加
      4. 商户提现 → 管理员审核
      5. 验证全流程数据一致
    Expected Result: 每步状态和金额正确
    Failure Indicators: 任何步骤失败
    Evidence: .sisyphus/evidence/task-19-e2e-flow.txt
  ```

  **Commit**: YES
  - Message: `test: end-to-end integration tests`
  - Files: `.sisyphus/evidence/task-19-*`

- [x] 20. 文档 — README、API 文档、部署指南

  **What to do**:
  - `README.md`：项目简介、技术栈、快速开始、协议接口文档
  - `docs/API.md`：管理后台和商户端 API 文档
  - `docs/DEPLOY.md`：部署指南（Docker、环境变量、HTTPS）
  - `docs/EASYPAY_PROTOCOL.md`：EasyPay 协议说明（面向接入商户）

  **Must NOT do**:
  - 不要写英文版本

  **Recommended Agent Profile**:
  - **Category**: `writing`
  - **Skills**: [`writing`]

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 5
  - **Blocked By**: Tasks 6-8（协议行为确定后）

  **Acceptance Criteria**:
  - [ ] README 包含一键启动步骤
  - [ ] EasyPay 协议文档含完整参数和签名说明

  **QA Scenarios**:

  ```
  Scenario: 按文档操作可成功启动
    Tool: Bash
    Steps:
      1. 按 README 步骤执行 docker compose up -d
      2. 健康检查通过
    Expected Result: 操作步骤与实际一致
    Evidence: .sisyphus/evidence/task-20-readme-verify.txt
  ```

  **Commit**: YES
  - Message: `docs: README, API documentation, deployment guide`
  - Files: `README.md`, `docs/API.md`, `docs/DEPLOY.md`, `docs/EASYPAY_PROTOCOL.md`

&zwj;

> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [ ] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists. For each "Must NOT Have": search codebase for forbidden patterns. Check evidence files exist in `.sisyphus/evidence/`. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [ ] F2. **Code Quality Review** — `unspecified-high`
  Run `go build ./...` + `go vet ./...`. Review all changed files for: hardcoded credentials, empty error handling, unexported but unused functions, AI slop patterns. Check frontend: `npm run build`.
  Output: `Build [PASS/FAIL] | Vet [PASS/FAIL] | Frontend [PASS/FAIL] | VERDICT`

- [ ] F3. **Real Manual QA** — `unspecified-high` (+ `playwright` skill)
  Start from clean Docker Compose. Execute EVERY QA scenario from EVERY task. Test cross-task integration. Test edge cases: duplicate out_trade_no, invalid sign, expired order. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | VERDICT`

- [ ] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff. Verify 1:1 — everything in spec was built, nothing beyond spec was built. Check "Must NOT do" compliance.
  Output: `Tasks [N/N compliant] | Contamination [CLEAN/N issues] | VERDICT`

---

## Commit Strategy

- **合并提交**: 全部完成后一次性 commit，message: `feat: epay payment aggregation platform v0.1`
- 中间阶段性提交（Wave 完成后）：`feat(wave-N): brief description`

---

## Success Criteria

### Verification Commands
```bash
# 启动
docker compose up -d

# 创建支付测试
curl -X POST http://localhost:8080/mapi.php \
  -d "pid=1001&type=alipay&out_trade_no=TEST001&name=Test&money=0.01&notify_url=http://localhost:8080/test/notify&sign=xxx&sign_type=MD5"

# 期望返回
# {"code":1,"trade_no":"...","qrcode":"..."}

# 管理后台
# http://localhost:3002 → 登录页 → 商户管理 → 创建商户

# 商户端
# http://localhost:3001 → 注册 → 查看订单 → 提现
```

### Final Checklist
- [ ] All "Must Have" present
- [ ] All "Must NOT Have" absent
- [ ] Docker Compose 一键启动
- [ ] EasyPay 协议完全兼容
- [ ] 支付宝/微信支付可正常创建和回调
- [ ] 费率自动计算正确
