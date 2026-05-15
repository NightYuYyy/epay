# EasyPay 支付协议对接文档

本文档面向商户开发者，说明如何通过 EasyPay 兼容协议对接 Epay 支付平台。商户只需拥有 PID 和 PKEY，即可快速接入支付宝和微信支付。

## 前置条件

| 项目 | 说明 |
|------|------|
| PID | 商户编号，由平台分配，用于标识商户身份 |
| PKEY | 商户密钥，用于签名计算，请妥善保管 |
| 接口地址 | 由平台提供，例如 `https://pay.yourdomain.com` |

> 商户通过管理后台注册后获取 PID 和 PKEY。PKEY 仅在注册时返回一次，遗失需联系管理员重新生成。

---

## 签名算法

### 签名步骤

1. 移除参数中的 `sign` 和 `sign_type` 字段
2. 移除值为空的参数
3. 将剩余参数按参数名 ASCII 升序排序
4. 拼接为 `key1=value1&key2=value2&...` 格式
5. 在末尾拼接 PKEY
6. 计算 MD5，取 32 位小写 Hex

### 伪代码

```
params = {pid: "1001", type: "alipay", out_trade_no: "20250101001", name: "测试商品", money: "100.00", notify_url: "https://...", return_url: "https://..."}

// 1. 移除 sign 和 sign_type，移除空值
// 2. 按 key 排序
sorted_keys = ["money", "name", "notify_url", "out_trade_no", "pid", "return_url", "type"]

// 3. 拼接 key=value
raw = "money=100.00&name=测试商品&notify_url=https://...&out_trade_no=20250101001&pid=1001&return_url=https://...&type=alipay"

// 4. 末尾拼接 PKEY
raw = raw + "your_pkey_here"

// 5. MD5
sign = MD5(raw)
```

### 代码示例

**PHP：**
```php
function easyPaySign($params, $pkey) {
    // 移除 sign 和 sign_type
    unset($params['sign']);
    unset($params['sign_type']);

    // 移除空值参数
    $params = array_filter($params, function($v) { return $v !== ''; });

    // 按 key 排序
    ksort($params);

    // 拼接
    $raw = '';
    foreach ($params as $key => $value) {
        $raw .= $key . '=' . $value . '&';
    }
    $raw = rtrim($raw, '&');

    // 末尾追加 PKEY
    $raw .= $pkey;

    return strtolower(md5($raw));
}
```

**Python：**
```python
import hashlib

def easy_pay_sign(params: dict, pkey: str) -> str:
    # 移除 sign 和 sign_type
    params = {k: v for k, v in params.items()
              if k not in ('sign', 'sign_type') and v != ''}

    # 按 key 排序并拼接
    raw = '&'.join(f'{k}={params[k]}' for k in sorted(params.keys()))

    # 末尾追加 PKEY
    raw += pkey

    return hashlib.md5(raw.encode()).hexdigest()
```

### 签名验证示例

请求参数：
```
pid=1001
type=alipay
out_trade_no=20250101001
name=测试商品
money=100.00
notify_url=https://myshop.com/notify
return_url=https://myshop.com/return
sign_type=MD5
sign=e8f7c2d1a3b4c5d6e7f8a9b0c1d2e3f4
```

假设 PKEY 为 `abc123def456abc123def456abc123de`，签名计算过程：

```
排序后参数：
money=100.00
name=测试商品
notify_url=https://myshop.com/notify
out_trade_no=20250101001
pid=1001
return_url=https://myshop.com/return
type=alipay

待签名字符串：
money=100.00&name=测试商品&notify_url=https://myshop.com/notify&out_trade_no=20250101001&pid=1001&return_url=https://myshop.com/return&type=alipayabc123def456abc123def456abc123de

MD5 = e8f7c2d1a3b4c5d6e7f8a9b0c1d2e3f4
```

---

## 接口一：创建订单（mapi.php）

商户端发起支付请求，返回支付链接或二维码。

### 请求地址

```
POST /mapi.php
```

Content-Type：`application/x-www-form-urlencoded`

### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| pid | int | 是 | 商户编号 |
| type | string | 是 | 支付方式：`alipay` / `wxpay` |
| out_trade_no | string | 是 | 商户订单号，需唯一 |
| notify_url | string | 是 | 异步通知地址，不能包含查询参数，不能指向内网 IP |
| return_url | string | 否 | 同步跳转地址，支付完成后用户跳转的页面 |
| name | string | 是 | 商品名称或订单描述 |
| money | string | 是 | 订单金额，单位元，保留两位小数，如 `100.00` |
| sign | string | 是 | MD5 签名 |
| sign_type | string | 否 | 签名类型，固定为 `MD5` |
| clientip | string | 否 | 客户端 IP 地址，可选 |
| device | string | 否 | 设备类型，传入 `mobile` 跳转移动端支付页 |
| cid | string | 否 | 自定义渠道 ID，可选 |

### 响应格式

```json
{
  "code": 1,
  "msg": "",
  "trade_no": "平台交易号",
  "payurl": "https://pay.example.com/alipay?order=xxx",
  "qrcode": "https://pay.example.com/alipay?order=xxx"
}
```

| 字段 | 说明 |
|------|------|
| code | 1 表示成功，其他为失败 |
| msg | 错误描述（失败时） |
| trade_no | 平台交易号 |
| payurl | 支付链接，跳转到此地址完成支付 |
| qrcode | 支付二维码地址（微信 Native 支付时有效） |

### 错误响应

```json
// 商户不存在
{ "code": -1, "msg": "商户不存在" }

// 商户已禁用
{ "code": -1, "msg": "商户已禁用" }

// 签名错误
{ "code": -3, "msg": "签名错误" }

// 参数缺失
{ "code": -1, "msg": "pid不能为空" }
{ "code": -1, "msg": "out_trade_no不能为空" }

// notify_url 校验
{ "code": -1, "msg": "notify_url不能包含查询参数" }
{ "code": -1, "msg": "notify_url不允许指向内网地址" }
```

### 请求示例

```bash
curl -X POST https://pay.yourdomain.com/mapi.php \
  -d "pid=1001" \
  -d "type=alipay" \
  -d "out_trade_no=20250101001" \
  -d "notify_url=https://myshop.com/notify" \
  -d "return_url=https://myshop.com/return" \
  -d "name=测试商品" \
  -d "money=100.00" \
  -d "sign=e8f7c2d1a3b4c5d6e7f8a9b0c1d2e3f4" \
  -d "sign_type=MD5"
```

### 幂等性说明

相同 `out_trade_no` 的重复请求会返回已有的支付信息，前提是参数（金额、支付方式）一致。如果参数不一致，会返回错误 `订单已存在且参数不一致`。

---

## 接口二：跳转支付（submit.php）

浏览器端直接跳转到此地址，自动创建订单并跳转到支付页。

### 请求地址

```
GET /submit.php?pid=1001&type=alipay&out_trade_no=...&money=100.00&name=...&notify_url=...&return_url=...&sign=...
```

### 请求参数

与 `mapi.php` 一致，但通过 URL 查询参数（GET 方式）传递。

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| pid | int | 是 | 商户编号 |
| type | string | 是 | 支付方式：`alipay` / `wxpay` |
| out_trade_no | string | 是 | 商户订单号 |
| notify_url | string | 是 | 异步通知地址 |
| return_url | string | 否 | 同步跳转地址 |
| name | string | 是 | 商品名称 |
| money | string | 是 | 订单金额，单位元 |
| sign | string | 是 | MD5 签名 |
| sign_type | string | 否 | 签名类型，固定为 `MD5` |
| device | string | 否 | 设备类型 |
| clientip | string | 否 | 客户端 IP |

### 响应

成功时返回 302 重定向到支付页面。失败时返回 JSON 响应（同 `mapi.php` 错误格式）。

### 使用方式

商户网站直接生成跳转链接：

```html
<!-- HTML 跳转 -->
<a href="https://pay.yourdomain.com/submit.php?pid=1001&type=alipay&out_trade_no=20250101001&money=100.00&name=测试商品&notify_url=https://myshop.com/notify&return_url=https://myshop.com/return&sign=e8f7c2d1a3b4c5d6e7f8a9b0c1d2e3f4&sign_type=MD5">
  支付宝支付
</a>
```

```php
// PHP 生成支付按钮
$params = [
    'pid' => '1001',
    'type' => 'alipay',
    'out_trade_no' => '20250101001',
    'notify_url' => 'https://myshop.com/notify',
    'return_url' => 'https://myshop.com/return',
    'name' => '测试商品',
    'money' => '100.00',
    'sign_type' => 'MD5',
];
$params['sign'] = easyPaySign($params, '你的PKEY');
$url = 'https://pay.yourdomain.com/submit.php?' . http_build_query($params);

echo '<a href="' . $url . '">去支付</a>';
```

---

## 接口三：查询订单（api.php）

查询订单的当前支付状态。

### 请求地址

```
POST /api.php?act=order
```

Content-Type：`application/x-www-form-urlencoded`

### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| act | string | 是 | 固定为 `order`，通过 URL 查询参数传递 |
| pid | int | 是 | 商户编号 |
| key | string | 是 | 商户 PKEY（明文传输） |
| out_trade_no | string | 二选一 | 商户订单号 |
| trade_no | string | 二选一 | 平台交易号 |

`out_trade_no` 和 `trade_no` 至少提供一个，`out_trade_no` 优先。

### 响应

```json
// 订单已支付
{ "code": 1, "status": 1, "money": "100.00" }

// 订单未支付
{ "code": 1, "status": 0, "money": "100.00" }

// 订单不存在
{ "code": -1, "msg": "订单不存在" }
```

| 字段 | 说明 |
|------|------|
| code | 1 表示查询成功，-1 表示失败 |
| status | 0 未支付，1 已支付 |
| msg | 错误描述（失败时） |
| money | 订单金额，单位元 |

### 请求示例

```bash
curl -X POST 'https://pay.yourdomain.com/api.php?act=order' \
  -d "pid=1001" \
  -d "key=你的PKEY" \
  -d "out_trade_no=20250101001"
```

---

## 异步通知（notify_url）

订单支付成功后，平台会向商户的 `notify_url` 发送 POST 通知。

### 通知方式

- 请求方式：`POST`
- Content-Type：`application/json`
- 超时时间：10 秒

### 通知参数

```json
{
  "out_trade_no": "20250101001",
  "trade_no": "2025011522001411111111111111",
  "money": "100.00",
  "status": "success",
  "paid_at": "2025-01-15T10:30:00Z"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| out_trade_no | string | 商户订单号 |
| trade_no | string | 平台交易号或上游交易号 |
| money | string | 实际支付金额，单位元 |
| status | string | 固定为 `success` |
| paid_at | string | 支付时间（RFC 3339 格式） |

### 响应要求

商户收到通知后，需返回 HTTP 200-299 状态码且响应体为 `success` 字符串表示成功接收。返回其他内容平台会认为通知失败。

```
HTTP/1.1 200 OK
Content-Type: text/plain

success
```

### 重试策略

平台在以下时间间隔重试，最多 5 次：

| 重试次数 | 间隔 |
|----------|------|
| 第 1 次 | 10 秒 |
| 第 2 次 | 60 秒 |
| 第 3 次 | 10 分钟 |
| 第 4 次 | 30 分钟 |
| 第 5 次 | 60 分钟 |

### 通知安全性

- notify_url 不能包含查询参数
- notify_url 不能指向内网地址（127.0.0.0/8、10.0.0.0/8、172.16.0.0/12、192.168.0.0/16）
- 商户应在收到通知后校验订单金额是否一致，防止伪造通知
- 建议商户在收到通知后调用 `api.php` 接口二次确认订单状态

---

## 订单状态说明

| 状态 | 说明 |
|------|------|
| PENDING | 待支付，订单已创建但用户尚未付款 |
| PAID | 已支付，用户已完成付款 |
| SETTLED | 已结算，订单金额已计入商户余额 |
| EXPIRED | 已过期，订单超过 30 分钟未支付自动过期 |
| CANCELLED | 已取消 |

## 错误码汇总

| code | 说明 |
|------|------|
| 1 | 成功 |
| -1 | 通用错误（详见 msg 字段） |
| -3 | 签名错误 |

常见错误消息：

| msg | 说明 |
|-----|------|
| pid不能为空 | 缺少商户编号 |
| pid格式无效 | 商户编号格式错误 |
| 商户不存在 | 商户编号未注册 |
| 商户已禁用 | 商户已被管理员禁用 |
| 签名错误 | sign 参数不正确 |
| out_trade_no不能为空 | 缺少商户订单号 |
| 订单已存在且参数不一致 | 重复的订单号但金额或支付方式不匹配 |
| notify_url不能包含查询参数 | 通知地址格式违规 |
| notify_url不允许指向内网地址 | 通知地址不能指向内网 IP |
| notify_url格式无效 | 通知地址格式错误 |
| 不支持的act | act 参数不是 `order` |
| key不正确 | PKEY 错误（api.php） |
| out_trade_no和trade_no不能同时为空 | 查询订单时两个参数都未提供 |
| 订单不存在 | 未找到对应订单 |

---

## 完整对接流程

### 流程图

```
商户系统                    Epay 平台                      支付宝/微信
    |                          |                              |
    |--- 1. mapi.php -------->|                              |
    |                          |--- 验证签名、创建订单 ------->|
    |<-- payurl + qrcode ------|                              |
    |                          |                              |
    |--- 2. 用户跳转 payurl -->|                              |
    |                          |--- 请求支付链接 ------------->|
    |                          |<-- 支付页面 -----------------|
    |                          |                              |
    |--- 3. 用户完成支付 ------|                              |
    |                          |<-- 异步回调 -----------------|
    |                          |--- 验证回调、更新订单 -------|
    |                          |                              |
    |<-- 4. notify_url POST ---|                              |
    |--- 5. 返回 "success" --->|                              |
    |                          |                              |
    |--- 6. api.php 查询 ----->|                              |
    |<-- status=1 确认已支付 ---|                              |
```

### 对接步骤

1. 商户调用 `mapi.php` 或引导用户访问 `submit.php` 创建订单
2. 用户跳转到支付页面完成支付
3. 平台收到支付回调，更新订单状态为已支付
4. 平台向商户 `notify_url` 发送异步通知
5. 商户返回 `success` 确认收到通知
6. 商户可调用 `api.php` 二次确认订单状态

### 最佳实践

- 生成订单时 `out_trade_no` 确保唯一，建议格式：`日期+商户ID+流水号`
- `notify_url` 不要包含查询参数，路径保持简洁
- 收到异步通知后先校验订单金额，再返回 `success`
- 对关键订单建议定时调用 `api.php` 轮询状态作为兜底
- 对账时以异步通知为主，`api.php` 查询为辅

---

## 限额说明

| 支付方式 | 单笔限额 | 说明 |
|----------|----------|------|
| 支付宝 | 以支付宝官方限制为准 | 通常借记卡单笔 5 万 |
| 微信支付 | 以微信支付官方限制为准 | 通常借记卡单笔 5 万 |

## 费率说明

| 费用类型 | 说明 |
|----------|------|
| 官方手续费 | 支付宝/微信官方收取的手续费 |
| 平台佣金 | 平台按商户费率计算的服务费 |
| 商户实收 | 订单金额 - 平台佣金 |

---

## 相关文档

- `API.md` — 管理后台和商户端 API 文档
- `README.md` — 项目总览与快速开始
