package easypay

import (
	"encoding/base64"
	"fmt"
	"html"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

func paymentQRCodeHTML(rawQRCode, payType, orderNo, subject string) (string, error) {
	rawQRCode = strings.TrimSpace(rawQRCode)
	if rawQRCode == "" {
		return "", fmt.Errorf("qrcode is empty")
	}

	png, err := qrcode.Encode(rawQRCode, qrcode.Medium, 288)
	if err != nil {
		return "", fmt.Errorf("encode qrcode: %w", err)
	}

	title := paymentQRCodeTitle(payType)
	escapedTitle := html.EscapeString(title)
	escapedOrderNo := html.EscapeString(strings.TrimSpace(orderNo))
	escapedSubject := html.EscapeString(strings.TrimSpace(subject))
	escapedQRCode := html.EscapeString(rawQRCode)
	image := base64.StdEncoding.EncodeToString(png)

	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      background: #f5f7fb;
      color: #1f2937;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    .card {
      width: min(92vw, 430px);
      padding: 28px;
      border-radius: 18px;
      background: #fff;
      box-shadow: 0 18px 50px rgba(15, 23, 42, .14);
      text-align: center;
    }
    h1 { margin: 0 0 10px; font-size: 22px; }
    .hint { margin: 0 0 22px; color: #6b7280; font-size: 14px; }
    .qr {
      width: 288px;
      height: 288px;
      max-width: 100%%;
      border: 1px solid #e5e7eb;
      border-radius: 14px;
      padding: 10px;
      background: #fff;
    }
    .meta {
      margin-top: 18px;
      padding: 12px;
      border-radius: 12px;
      background: #f9fafb;
      color: #4b5563;
      font-size: 13px;
      line-height: 1.7;
      text-align: left;
      word-break: break-all;
    }
    .raw { margin-top: 10px; color: #9ca3af; font-size: 12px; }
  </style>
</head>
<body>
  <main class="card">
    <h1>%s</h1>
    <p class="hint">请使用对应支付 App 扫码完成支付</p>
    <img class="qr" src="data:image/png;base64,%s" alt="支付二维码">
    <div class="meta">
      <div><strong>订单号：</strong>%s</div>
      <div><strong>商品：</strong>%s</div>
      <div class="raw"><strong>二维码内容：</strong>%s</div>
    </div>
  </main>
</body>
</html>`, escapedTitle, escapedTitle, image, escapedOrderNo, escapedSubject, escapedQRCode), nil
}

func paymentQRCodeTitle(payType string) string {
	switch strings.ToLower(strings.TrimSpace(payType)) {
	case "alipay":
		return "支付宝扫码支付"
	case "wxpay", "wechat", "weixin":
		return "微信支付"
	default:
		return "扫码支付"
	}
}
