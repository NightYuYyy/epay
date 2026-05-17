// Package easypay implements rainbow-epay compatible EasyPay protocol handlers.
//
// This file provides the signature primitives shared by the request side
// (verifying merchant signatures) and the response side (signing notifications
// and API responses). The algorithm is byte-for-byte compatible with
// rainbow-epay's `lib\Payment::getSignContent` / `makeSign` / `verifySign`:
//
//  1. Sort keys ascending by key name
//  2. Skip `sign` / `sign_type` / empty values
//  3. Concatenate as `k1=v1&k2=v2&...`
//  4. For MD5: append merchant pkey, md5 lowercase hex
//  5. For RSA: SHA-256 + PKCS#1 v1.5; base64 of signature bytes
package easypay

import (
	"crypto"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// SignTypeMD5 / SignTypeRSA enumerate the supported signature algorithms.
const (
	SignTypeMD5 = "MD5"
	SignTypeRSA = "RSA"
)

// ErrInvalidSignType indicates an unknown sign_type value.
var ErrInvalidSignType = errors.New("invalid sign_type")

// BuildSignContent returns the canonical string used as input to the signing
// algorithm. It matches rainbow-epay's `Payment::getSignContent` exactly:
// keys sorted ascending; the literal `sign` / `sign_type` keys are excluded;
// values that are empty after trimming whitespace are excluded (so a literal
// "0" string IS included — only blank strings are skipped).
func BuildSignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || k == "sign_type" {
			continue
		}
		if strings.TrimSpace(v) == "" {
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
	return buf.String()
}

// SignMD5 returns the lowercase 32-char hex MD5 of (sign_content + pkey).
func SignMD5(params map[string]string, pkey string) string {
	content := BuildSignContent(params) + pkey
	sum := md5.Sum([]byte(content))
	return hex.EncodeToString(sum[:])
}

// VerifyMD5 checks the supplied `sign` against the computed signature using
// constant-time comparison.
func VerifyMD5(params map[string]string, pkey, sign string) bool {
	expected := SignMD5(params, pkey)
	return hmac.Equal([]byte(expected), []byte(sign))
}

// SignRSA signs the canonical content with the supplied PEM-encoded RSA
// private key using SHA-256 + PKCS#1 v1.5 and returns the base64 of the raw
// signature bytes. This matches rainbow-epay's `openssl_sign(..., SHA256)`
// + `base64_encode`.
func SignRSA(params map[string]string, privateKeyPEMOrBase64 string) (string, error) {
	priv, err := ParseRSAPrivateKey(privateKeyPEMOrBase64)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	content := BuildSignContent(params)
	hashed := sha256.Sum256([]byte(content))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("rsa sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyRSA verifies that `sign` (base64) was produced by the holder of the
// private key matching the supplied PEM/base64 public key.
func VerifyRSA(params map[string]string, publicKeyPEMOrBase64, sign string) error {
	pub, err := ParseRSAPublicKey(publicKeyPEMOrBase64)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	content := BuildSignContent(params)
	hashed := sha256.Sum256([]byte(content))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed[:], sigBytes); err != nil {
		return fmt.Errorf("rsa verify: %w", err)
	}
	return nil
}

// VerifyParamSign auto-detects the algorithm from params["sign_type"] (defaults
// to MD5 when absent, matching rainbow's behavior). For MD5 it uses pkey; for
// RSA it uses the merchant public key.
//
// Returns nil on success. The caller should map errors to the canonical
// rainbow error messages ("签名错误" / "RSA签名校验失败").
func VerifyParamSign(params map[string]string, pkey, publicKeyPEM string) error {
	sign, ok := params["sign"]
	if !ok || sign == "" {
		return errors.New("缺少签名参数")
	}
	signType := strings.ToUpper(strings.TrimSpace(params["sign_type"]))
	if signType == "" {
		signType = SignTypeMD5
	}
	switch signType {
	case SignTypeMD5:
		if !VerifyMD5(params, pkey, sign) {
			return errors.New("签名错误")
		}
		return nil
	case SignTypeRSA:
		if publicKeyPEM == "" {
			return errors.New("签名校验失败，商户公钥错误")
		}
		if err := VerifyRSA(params, publicKeyPEM, sign); err != nil {
			return errors.New("RSA签名校验失败")
		}
		return nil
	default:
		return ErrInvalidSignType
	}
}

// ---- Key parsing helpers --------------------------------------------------

// ParseRSAPublicKey accepts a PEM-encoded public key OR a raw base64 string
// (as rainbow's `base64ToPem` would accept) and returns the *rsa.PublicKey.
func ParseRSAPublicKey(s string) (*rsa.PublicKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty public key")
	}
	pemBytes := normalizePEM(s, "PUBLIC KEY")
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM block")
	}
	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
		return nil, errors.New("not an RSA public key")
	}
	if pub, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return pub, nil
	}
	return nil, errors.New("unsupported public key format")
}

// ParseRSAPrivateKey accepts PEM or raw base64.
func ParseRSAPrivateKey(s string) (*rsa.PrivateKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty private key")
	}
	pemBytes := normalizePEM(s, "PRIVATE KEY")
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("not an RSA private key")
	}
	return nil, errors.New("unsupported private key format")
}

// normalizePEM accepts:
//   - PEM-formatted string ("-----BEGIN ...-----\n...")
//   - raw base64 of the DER bytes
//
// And returns canonical PEM bytes with the given label.
func normalizePEM(s, label string) []byte {
	if strings.Contains(s, "BEGIN") {
		return []byte(s)
	}
	// Strip whitespace then re-wrap as PEM with 64-char lines.
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, s)
	var buf strings.Builder
	buf.WriteString("-----BEGIN ")
	buf.WriteString(label)
	buf.WriteString("-----\n")
	for i := 0; i < len(clean); i += 64 {
		end := i + 64
		if end > len(clean) {
			end = len(clean)
		}
		buf.WriteString(clean[i:end])
		buf.WriteByte('\n')
	}
	buf.WriteString("-----END ")
	buf.WriteString(label)
	buf.WriteString("-----\n")
	return []byte(buf.String())
}
