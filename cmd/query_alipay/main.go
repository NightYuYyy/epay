package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"epay/internal/provider/alipay"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: query <out_trade_no>\n")
		os.Exit(1)
	}
	outTradeNo := strings.TrimSpace(os.Args[1])

	// Read config from env
	config := map[string]string{}
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			config[parts[0]] = parts[1]
		}
	}

	apCfg := map[string]string{
		"appId":           config["A_APP_ID"],
		"privateKey":      config["A_PRIVATE_KEY"],
		"publicKey":       config["A_PUBLIC_KEY"],
		"alipayPublicKey": config["A_PUBLIC_KEY"],
		"notifyUrl":       config["A_NOTIFY_URL"],
		"returnUrl":       config["A_RETURN_URL"],
	}
	if strings.EqualFold(config["A_PRODUCTION"], "true") {
		apCfg["production"] = "true"
	}

	ap, err := alipay.NewAlipay("query", apCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := ap.QueryOrder(ctx, outTradeNo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("TradeNo=%s\nStatus=%s\nAmount=%.2f\nPaidAt=%s\n", resp.TradeNo, resp.Status, resp.Amount, resp.PaidAt)
}
