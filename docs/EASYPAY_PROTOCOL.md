# EasyPay 支付协议对接文档（兼容彩虹 EasyPay）

本文档面向商户开发者，说明如何通过 EasyPay 兼容协议对接 Epay 支付平台。协议与彩虹 EasyPay 完全兼容：商户端代码无需改动即可对接。

## 前置条件

| 项目 | 说明 |
|------|------|
| PID | 商户编号，由平台分配 |
| PKEY | MD5 商户密钥，用于 MD5 签名（`keytype=0`） |
| 公钥/私钥 | RSA 商户使用（`keytype=1`），上传商户公钥到平台 |
| 接口地址 | 由平台提供，例如 `https://pay.yourdomain.com` |

---

## 一、签名算法

支持两种算法，由商户 `keytype` 决定：

- `keytype=0`（默认）—— MD5
- `keytype=1` —— RSA-SHA256-PKCS1v15

签名生成步骤一致：

1. 移除参数中的 `sign` 与 `sign_type` 字段
2. 移除值为空（trim 后空字符串）的参数；字符串 `"0"` **不**视为空
3. 按参数名 ASCII 升序排序
4. 拼接为 `key1=value1&key2=value2&...`
5. 计算签名：
   - MD5：`md5(content + pkey)`，输出 32 位小写 hex
   - RSA：`base64( RSA_SHA256( content, 平台私钥 ) )`（下行）或商户私钥（上行）

### MD5 PHP 参考实现

```php
function easyPaySign($params, $pkey) {
    unset($params['sign'], $params['sign_type']);
    $params = array_filter($params, fn($v) => trim($v) !== '');
    ksort($params);
    $raw = '';
    foreach ($params as $k => $v) $raw .= "$k=$v&";
    return strtolower(md5(rtrim($raw, '&') . $pkey));
}
```

### RSA Python 参考实现

```python
from base64 import b64encode
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding

def rsa_sign(params: dict, private_key_pem: bytes) -> str:
    params = {k: v for k, v in params.items()
              if k not in ('sign', 'sign_type') and str(v).strip() != ''}
    content = '&'.join(f'{k}={params[k]}' for k in sorted(params))
    key = serialization.load_pem_private_key(private_key_pem, password=None)
    sig = key.sign(content.encode(),
                   padding.PKCS1v15(),
                   hashes.SHA256())
    return b64encode(sig).decode()
```

### 固定向量

```
params  = {pid:"1001", out_trade_no:"20250101", name:"hi", money:"100.00", sign_type:"MD5"}
pkey    = "abc123"
content = "money=100.00&name=hi&out_trade_no=20250101&pid=1001"
md5(content + pkey) = 7d485d80eaf0b05747315811496959cb
```

---

## 二、接口一：创建订单（mapi.php）

`POST /mapi.php`，Content-Type：`application/x-www-form-urlencoded`，返回 JSON。

### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| pid | int | 是 | 商户编号 |
| type | string | 是¹ | 支付方式：`alipay` / `wxpay` |
| out_trade_no | string | 是 | 商户订单号，需唯一；正则 `[a-zA-Z0-9._\-|]+` |
| notify_url | string | 是 | 异步通知地址；不能含查询参数；不能指向内网 |
| return_url | string | 是 | 同步跳转地址 |
| name | string | 是 | 商品名称（≤127 字符） |
| money | string | 是 | 订单金额，单位元，正则 `[0-9.]+` 且 > 0 |
| clientip | string | 否 | 客户端 IP |
| device | string | 否 | `pc` / `mobile` / `qq` / `wechat` / `alipay` / `app`，默认 `pc` |
| method | string | 否 | `web` / `jump` / `jsapi` / `scan` |
| param | string | 否 | 业务回传参数，原样回传到 notify_url |
| sitename | string | 否 | 站点名称 |
| sub_openid | string | 是² | JSAPI 支付时必填 |
| sub_appid | string | 是³ | 微信 JSAPI 支付时必填 |
| auth_code | string | 是⁴ | 付款码（scan）支付时必填 |
| sign | string | 是 | 签名 |
| sign_type | string | 是 | `MD5` / `RSA` |

¹ 仅 `method=scan` 可省略。
² 当 `method=jsapi` 时必填。
³ 当 `method=jsapi` 且 `type=wxpay` 时必填。
⁴ 当 `method=scan` 时必填。

### 响应

```json
{
  "code": 1,
  "msg": "",
  "trade_no": "20250101153012X1234",
  "payurl": "https://pay.example.com/alipay?order=20250101001",
  "qrcode": "https://pay.example.com/alipay?order=20250101001"
}
```

### 错误码

| code | 含义 |
|---|---|
| 1 | 成功 |
| -1 | 通用错误（详见 `msg`） |
| -2 | 业务错误（如商户未开退款） |
| -3 | 签名错误 / 商户身份错误 |
| -4 | 参数缺失 |
| -5 | 接口不存在 / URL Error |

常见错误消息（与彩虹一一对应）：
- `商户ID不能为空`
- `商户不存在！`
- `商户已被封禁，无法支付！`
- `该商户只能使用RSA签名类型`
- `签名错误` / `RSA签名校验失败`
- `订单号(out_trade_no)不能为空`
- `订单号(out_trade_no)格式不正确`
- `通知地址(notify_url)不能为空`
- `回调地址(return_url)不能为空`
- `商品名称(name)不能为空`
- `金额不合法`
- `支付方式(type)不能为空`
- `notify_url不能包含查询参数`
- `notify_url不允许指向内网地址`
- `该订单(xxx)已完成支付，请勿重复发起支付`
- `该订单(xxx)支付参数有变化，请更换订单号重新发起支付`

---

## 三、接口二：跳转支付（submit.php）

`GET /submit.php` 或 `POST /submit.php`，参数与 `mapi.php` 一致。成功时返回 302 重定向到支付页。

```html
<a href="https://pay.yourdomain.com/submit.php?pid=1001&type=alipay&out_trade_no=20250101001&money=100.00&name=测试&notify_url=https://shop.com/notify&return_url=https://shop.com/return&sign=...&sign_type=MD5">
  支付宝支付
</a>
```

---

## 四、接口三：查询接口（api.php）

`GET /api.php?act=<动作>`（部分动作支持 POST）。

### 4.1 `act=query` — 商户概览

```
GET /api.php?act=query&pid=1001&key=<PKEY>
```

响应：

```json
{
  "code": 1,
  "pid": 1001,
  "key": "<PKEY>",
  "active": 1,
  "money": "1234.56",
  "type": 0,
  "account": "",
  "username": "示例商户",
  "orders": 250,
  "orders_today": 8,
  "orders_lastday": 12
}
```

### 4.2 `act=settle` — 结算/提现记录

```
GET /api.php?act=settle&pid=1001&key=<PKEY>&limit=10&offset=0
```

`limit` 最大 50。

### 4.3 `act=order` — 单笔订单

商户模式：

```
GET /api.php?act=order&pid=1001&key=<PKEY>&out_trade_no=20250101001
# 或 trade_no=...
```

平台模式（跨商户）：

```
GET /api.php?act=order&trade_no=20250101153012X1234&sign=<md5(SYS_KEY+trade_no+SYS_KEY)>
```

响应：

```json
{
  "code": 1,
  "msg": "succ",
  "trade_no": "20250101153012X1234",
  "out_trade_no": "20250101001",
  "api_trade_no": "20250101001…ALIPAY",
  "type": "alipay",
  "pid": 1001,
  "addtime": "2025-01-01 15:30:12",
  "endtime": "2025-01-01 15:31:45",
  "name": "测试商品",
  "money": "100.00",
  "param": "biz=abc",
  "buyer": "buyer@example.com",
  "status": 1,
  "payurl": "https://pay.example.com/...",
  "refundmoney": "0.00"
}
```

`status`：`0` 未支付，`1` 已支付，`2` 已取消。

### 4.4 `act=orders` — 订单列表

```
GET /api.php?act=orders&pid=1001&key=<PKEY>&limit=10&offset=0&status=1
```

### 4.5 `act=refund` — 自助退款

`POST /api.php?act=refund`，表单参数：

| 参数名 | 必填 | 说明 |
|---|---|---|
| pid | 是 | 商户编号 |
| key | 是 | PKEY |
| trade_no / out_trade_no | 二选一 | 订单号 |
| money | 是 | 退款金额 |
| out_refund_no | 否 | 商户退款单号（幂等键） |

> ⚠️ 当前版本协议层已支持，上游退款 SDK 集成进行中。响应会返回 `code=-3, msg=暂不支持自动退款，请联系平台手动处理`，但退款工单已落库。

### 4.6 `act=refundquery` — 退款查询

```
GET /api.php?act=refundquery&pid=1001&key=<PKEY>&refund_no=<RF...>
# 或 out_refund_no=...
```

---

## 五、RSA `s=path` API（API_INIT 模式）

`POST /api.php?s=<class>/<func>`，强制 RSA 签名 + `timestamp` 防重放（±300s）。

支持路由：

| 路由 | 等价于 |
|---|---|
| `pay/create` | `mapi.php`（RSA 模式） |
| `pay/query` | `act=order` |
| `pay/refund` | `act=refund` |
| `pay/refundquery` | `act=refundquery` |

### 请求示例

```bash
curl -X POST 'https://pay.yourdomain.com/api.php?s=pay/create' \
  -d 'pid=1001' \
  -d 'type=alipay' \
  -d 'out_trade_no=20250101001' \
  -d 'notify_url=https://shop.com/notify' \
  -d 'return_url=https://shop.com/return' \
  -d 'name=测试' \
  -d 'money=100.00' \
  -d 'clientip=1.2.3.4' \
  -d 'method=jump' \
  -d 'timestamp=1700000000' \
  -d 'sign_type=RSA' \
  -d 'sign=<RSA_SIGN>'
```

### 响应示例

```json
{
  "code": 0,
  "trade_no": "20250101153012X1234",
  "pay_type": "jump",
  "pay_info": "https://pay.example.com/...",
  "timestamp": "1700000123",
  "sign_type": "RSA",
  "sign": "<base64 RSA 平台私钥签名>"
}
```

商户使用**平台公钥**验证响应签名，规则同上行签名（排序、拼接、SHA256+PKCS1v15）。

平台公钥获取：联系平台管理员，或通过约定的密钥下发渠道。

---

## 六、异步通知（notify_url）

订单支付成功后，平台向商户 `notify_url` 发送 **GET 请求**（rainbow EasyPay 标准），URL 上附加签名参数。

### 通知字段

| 字段 | 类型 | 说明 |
|------|------|------|
| pid | int | 商户编号 |
| trade_no | string | 平台交易号 |
| out_trade_no | string | 商户订单号 |
| type | string | 支付方式 |
| name | string | 商品名称 |
| money | float | 金额 |
| trade_status | string | 固定 `TRADE_SUCCESS` |
| param | string | 业务回传参数（如有） |
| api_trade_no | string | 上游交易号（如有） |
| buyer | string | 付款人账号（如有） |
| sign | string | 签名 |
| sign_type | string | `MD5` 或 `RSA` |
| timestamp | string | RSA 模式才会出现 |

### 签名规则

- `version=0`（标准接口创建的订单）→ MD5，使用商户 PKEY
- `version=1`（RSA `s=path` 创建的订单）→ RSA，使用**平台私钥**签名；商户用**平台公钥**验签

### 商户响应

```
HTTP/1.1 200 OK
Content-Type: text/plain

success
```

平台收到 `success` 文本视为成功；否则按重试节奏重投：

| 重试 | 间隔 |
|---|---|
| 第 1 次 | 10 秒 |
| 第 2 次 | 60 秒 |
| 第 3 次 | 10 分钟 |
| 第 4 次 | 30 分钟 |
| 第 5 次 | 60 分钟 |

### 安全建议

- `notify_url` 不能包含查询参数（平台拒绝）
- `notify_url` 不能指向内网地址（平台拒绝）
- 收到通知后**先验签**，再核对订单金额
- 对关键订单建议轮询 `act=order` 二次确认

---

## 七、订单状态

平台内部状态映射到协议 `status` 字段：

| 内部状态 | 协议 status | 说明 |
|---|---|---|
| PENDING | 0 | 待支付 |
| PAID | 1 | 已支付 |
| SETTLED | 1 | 已结算（订单维度仍视为已支付） |
| EXPIRED | 0 | 已过期，可作未支付处理 |
| CANCELLED | 2 | 已取消 |

订单超过 10 天后不允许复用 `out_trade_no`，必须使用新订单号。

---

## 八、完整对接流程

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
    |<-- 4. notify_url GET ----|                              |
    |--- 5. 返回 "success" --->|                              |
    |                          |                              |
    |--- 6. api.php 查询 ----->|                              |
    |<-- status=1 确认已支付 ---|                              |
```

---

## 九、与彩虹 EasyPay 的差异

本平台目标是**协议层**完全兼容彩虹 EasyPay。彩虹中以下能力暂未实现（不影响协议层对接）：

- 多通道路由 / 通道费率 (`lib\Channel`)
- 商户加费模式 `mode=1` 的费用调整
- 分润 (`profits`) / 分账接收人 (`pre_psreceiver`)
- 域名白名单 (`pre_domain`)
- 风控关键词 (`pre_risk`)、IP 限流、实名认证
- 真实付款码 / JSAPI / 小程序 urlscheme 上游对接
- `act=transfer` 转账接口、`gold.php`、`getshop.php`、`wework.php`
- 安全验证页 (`txprotect.php`、`checkPayVerifyOpen`)

`act=refund` 协议层已实现，上游 SDK 退款功能进行中。

---

## 十、相关文档

- `API.md` — 管理后台和商户端 API 文档
- `DEPLOY.md` — 部署指南
- `README.md` — 项目总览
