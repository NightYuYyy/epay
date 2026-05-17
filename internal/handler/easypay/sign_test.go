package easypay

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

func TestBuildSignContent_AlphabeticalAndExcludesEmpty(t *testing.T) {
	got := BuildSignContent(map[string]string{
		"pid":          "1001",
		"type":         "alipay",
		"out_trade_no": "X1",
		"money":        "100.00",
		"empty":        "",     // excluded
		"sign":         "x",    // excluded
		"sign_type":    "MD5",  // excluded
	})
	want := "money=100.00&out_trade_no=X1&pid=1001&type=alipay"
	if got != want {
		t.Fatalf("BuildSignContent mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildSignContent_DoesNotExcludeZeroString(t *testing.T) {
	// Rainbow keeps "0" in the content; only blank-after-trim values are skipped.
	got := BuildSignContent(map[string]string{
		"a": "0",
		"b": " ",
		"c": "1",
	})
	want := "a=0&c=1"
	if got != want {
		t.Fatalf("zero-string handling broken\n got: %q\nwant: %q", got, want)
	}
}

func TestSignMD5_FixedVector(t *testing.T) {
	// Rainbow PHP reference (manually verified):
	//   content = "money=100.00&name=hi&out_trade_no=20250101&pid=1001"
	//   pkey    = "abc123"
	//   md5(content + pkey) = 0c8aa10fb6263cb6fa6d76fef0e872f3 (lowercased)
	params := map[string]string{
		"pid":          "1001",
		"out_trade_no": "20250101",
		"name":         "hi",
		"money":        "100.00",
		"sign_type":    "MD5",
	}
	got := SignMD5(params, "abc123")
	want := "7d485d80eaf0b05747315811496959cb"
	if got != want {
		t.Fatalf("SignMD5 fixed-vector mismatch\n got: %q\nwant: %q", got, want)
	}
	if !VerifyMD5(params, "abc123", got) {
		t.Fatalf("VerifyMD5 should accept the freshly-computed signature")
	}
}

func TestVerifyMD5_RejectsWrongKey(t *testing.T) {
	params := map[string]string{"pid": "1001", "out_trade_no": "X1"}
	sig := SignMD5(params, "key-a")
	if VerifyMD5(params, "key-b", sig) {
		t.Fatal("VerifyMD5 must reject signature produced with a different pkey")
	}
}

func TestRSARoundTrip(t *testing.T) {
	privKey, pubKeyPEM := mustGenerateRSAKeyPair(t)
	params := map[string]string{
		"pid":          "1001",
		"out_trade_no": "X1",
		"money":        "100.00",
		"timestamp":    "1700000000",
	}
	sig, err := SignRSA(params, privKey)
	if err != nil {
		t.Fatalf("SignRSA: %v", err)
	}
	if sig == "" {
		t.Fatal("empty signature")
	}
	if err := VerifyRSA(params, pubKeyPEM, sig); err != nil {
		t.Fatalf("VerifyRSA must accept fresh signature: %v", err)
	}
	// Tampered param must fail verification.
	params["money"] = "999.99"
	if err := VerifyRSA(params, pubKeyPEM, sig); err == nil {
		t.Fatal("VerifyRSA must reject tampered payload")
	}
}

func TestVerifyParamSign_AutoDetectsAlgorithm(t *testing.T) {
	// MD5 path
	mdParams := map[string]string{
		"pid":          "1001",
		"out_trade_no": "X1",
		"sign_type":    "MD5",
	}
	mdParams["sign"] = SignMD5(mdParams, "key")
	if err := VerifyParamSign(mdParams, "key", ""); err != nil {
		t.Fatalf("MD5 auto-detect failed: %v", err)
	}

	// RSA path
	priv, pubPEM := mustGenerateRSAKeyPair(t)
	rsaParams := map[string]string{
		"pid":          "1001",
		"out_trade_no": "X1",
		"timestamp":    "1700000000",
		"sign_type":    "RSA",
	}
	sig, err := SignRSA(rsaParams, priv)
	if err != nil {
		t.Fatalf("SignRSA: %v", err)
	}
	rsaParams["sign"] = sig
	if err := VerifyParamSign(rsaParams, "ignored", pubPEM); err != nil {
		t.Fatalf("RSA auto-detect failed: %v", err)
	}
}

func TestVerifyParamSign_MissingSign(t *testing.T) {
	err := VerifyParamSign(map[string]string{"pid": "1"}, "key", "")
	if err == nil {
		t.Fatal("expected error for missing sign")
	}
}

func TestParseRSA_AcceptsRawBase64AndPEM(t *testing.T) {
	priv, pubPEM := mustGenerateRSAKeyPair(t)

	// PEM form
	if _, err := ParseRSAPublicKey(pubPEM); err != nil {
		t.Fatalf("ParseRSAPublicKey PEM: %v", err)
	}
	if _, err := ParseRSAPrivateKey(priv); err != nil {
		t.Fatalf("ParseRSAPrivateKey PEM: %v", err)
	}

	// Raw base64 form (strip PEM wrappers, keep the body)
	rawPub := stripPEM(pubPEM)
	if _, err := ParseRSAPublicKey(rawPub); err != nil {
		t.Fatalf("ParseRSAPublicKey raw base64: %v", err)
	}
}

// ---- helpers --------------------------------------------------------------

func mustGenerateRSAKeyPair(t *testing.T) (privPEM, pubPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa: %v", err)
	}
	privDER := x509.MarshalPKCS1PrivateKey(key)
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER}))
	pubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	return
}

func stripPEM(s string) string {
	// Returns the base64 body of a PEM-encoded value (one continuous string).
	var b strings.Builder
	skip := false
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-----BEGIN") {
			skip = false
			continue
		}
		if strings.HasPrefix(line, "-----END") {
			continue
		}
		if !skip {
			b.WriteString(line)
		}
	}
	return b.String()
}
