// Package easypay implements the EasyPay protocol handler (server-side).
// Merchants call these endpoints to create payments, redirect users,
// and query order status via the standard EasyPay API.
package easypay

import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/hex"
	"sort"
	"strings"
)

// EasyPaySign computes the MD5 signature for the given parameters using the
// EasyPay algorithm:
//
//  1. Remove the "sign" and "sign_type" keys and any keys with empty values
//  2. Sort remaining keys alphabetically
//  3. Build the string: key1=value1&key2=value2&... + pkey
//  4. MD5 hash the concatenated string → lowercase hex (32 chars)
func EasyPaySign(params map[string]string, pkey string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(params[k])
	}
	buf.WriteString(pkey)

	hash := md5.Sum([]byte(buf.String()))
	return hex.EncodeToString(hash[:])
}

// EasyPayVerifySign compares the computed signature for the given params
// against the provided sign using constant-time comparison (HMAC.Equal)
// to prevent timing attacks.
func EasyPayVerifySign(params map[string]string, pkey, sign string) bool {
	return hmac.Equal([]byte(EasyPaySign(params, pkey)), []byte(sign))
}
