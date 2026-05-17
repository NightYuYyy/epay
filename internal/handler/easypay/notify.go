package easypay

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"epay/ent"
)

// MerchantNotifyRetryIntervals defines the backoff schedule for asynchronous
// notify_url callbacks. Matches the public protocol contract: 10s / 60s /
// 10min / 30min / 60min (≈ 5 retries total = 6 attempts including the initial).
var MerchantNotifyRetryIntervals = []time.Duration{
	10 * time.Second,
	60 * time.Second,
	10 * time.Minute,
	30 * time.Minute,
	60 * time.Minute,
}

// NotifyConfig captures the inputs required to build & dispatch a notify_url
// callback. Either the platform RSA key (for version=1 orders) or the merchant
// pkey (for version=0 MD5 orders) is needed depending on the order's version.
type NotifyConfig struct {
	PlatformRSAPrivateKey string
	HTTPClient            *http.Client
}

// BuildNotifyURL constructs the full rainbow-compatible notify URL for the
// given order. Algorithm matches rainbow-epay's `creat_callback`:
//
//   - Fixed fields: pid, trade_no, out_trade_no, type, name, money,
//     trade_status="TRADE_SUCCESS"
//   - Conditional fields (only when non-empty): param, api_trade_no, buyer
//   - When version == 1 → RSA + timestamp; otherwise MD5 with merchant pkey
//   - Append the query string to ord.NotifyURL (preserving existing `?`/`&`)
//
// The merchant argument provides Pid + Pkey + (optionally) the merchant's
// notify_url default; the order's NotifyURL takes precedence.
func BuildNotifyURL(ord *ent.Order, merch *ent.Merchant, platformPrivKey string) (string, error) {
	if ord == nil {
		return "", fmt.Errorf("nil order")
	}
	notifyURL := strings.TrimSpace(ord.NotifyURL)
	if notifyURL == "" {
		return "", fmt.Errorf("notify_url empty")
	}
	if merch == nil {
		return "", fmt.Errorf("nil merchant")
	}

	params := map[string]string{
		"pid":          strconv.Itoa(merch.Pid),
		"trade_no":     ord.TradeNo,
		"out_trade_no": ord.OrderNo,
		"type":         string(ord.Type),
		"name":         firstNonBlank(ord.Name, ord.OrderNo),
		"money":        formatMoneyTwo(ord.Amount),
		"trade_status": "TRADE_SUCCESS",
	}
	if ord.Param != "" {
		params["param"] = ord.Param
	}
	if ord.APITradeNo != "" {
		params["api_trade_no"] = ord.APITradeNo
	}
	if ord.Buyer != "" {
		params["buyer"] = ord.Buyer
	}

	if ord.Version == 1 {
		params["timestamp"] = strconv.FormatInt(time.Now().Unix(), 10)
		params["sign_type"] = SignTypeRSA
		sign, err := SignRSA(params, platformPrivKey)
		if err != nil {
			return "", fmt.Errorf("notify rsa sign: %w", err)
		}
		params["sign"] = sign
	} else {
		params["sign"] = SignMD5(params, merch.Pkey)
		params["sign_type"] = SignTypeMD5
	}

	return appendQueryParams(notifyURL, params), nil
}

// DispatchNotify sends the notify GET request and retries on transient failure
// according to MerchantNotifyRetryIntervals. Returns true when the merchant
// returned HTTP 2xx with body == "success" (rainbow contract).
func DispatchNotify(ctx context.Context, client *http.Client, fullURL string, orderRef string) bool {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	for attempt := 0; attempt <= len(MerchantNotifyRetryIntervals); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(MerchantNotifyRetryIntervals[attempt-1]):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			log.Printf("[easypay] notify %s build request: %v", orderRef, err)
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[easypay] notify %s attempt %d transport error: %v", orderRef, attempt+1, err)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.TrimSpace(string(body)) == "success" {
			return true
		}
		log.Printf("[easypay] notify %s attempt %d failed: status=%d body=%q",
			orderRef, attempt+1, resp.StatusCode, truncate(string(body), 200))
	}
	return false
}

// appendQueryParams concatenates the params (already-signed, in canonical map
// order — note rainbow uses PHP's http_build_query which preserves insertion
// order, but since the merchant only validates by signature this doesn't
// affect verification. We emit alphabetical for stability).
func appendQueryParams(rawURL string, params map[string]string) string {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	query := values.Encode()
	if strings.Contains(rawURL, "?") {
		return rawURL + "&" + query
	}
	return rawURL + "?" + query
}

func formatMoneyTwo(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
