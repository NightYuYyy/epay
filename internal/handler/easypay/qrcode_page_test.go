package easypay

import (
	"strings"
	"testing"
)

func TestPaymentQRCodeHTMLRendersScannableQRCodePage(t *testing.T) {
	html, err := paymentQRCodeHTML("weixin://wxpay/bizpayurl?pr=test", "wxpay", "ORDER123", "测试商品")
	if err != nil {
		t.Fatalf("paymentQRCodeHTML returned error: %v", err)
	}

	for _, want := range []string{
		"微信支付",
		"ORDER123",
		"data:image/png;base64,",
		"weixin://wxpay/bizpayurl?pr=test",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html does not contain %q:\n%s", want, html)
		}
	}
}
