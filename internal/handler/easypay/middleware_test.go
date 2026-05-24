package easypay

import "testing"

func TestValidateNotifyURLRejectsIPv6Loopback(t *testing.T) {
	if msg := ValidateNotifyURL("https://[::1]/notify"); msg == "" {
		t.Fatal("ValidateNotifyURL accepted IPv6 loopback notify_url")
	}
}

func TestValidateNotifyURLRejectsLinkLocalMetadataAddress(t *testing.T) {
	if msg := ValidateNotifyURL("http://169.254.169.254/latest/meta-data"); msg == "" {
		t.Fatal("ValidateNotifyURL accepted link-local metadata notify_url")
	}
}

func TestValidateNotifyURLRejectsIPv4MappedLoopback(t *testing.T) {
	if msg := ValidateNotifyURL("http://[::ffff:127.0.0.1]/notify"); msg == "" {
		t.Fatal("ValidateNotifyURL accepted IPv4-mapped loopback notify_url")
	}
}

func TestValidateNotifyURLRejectsCGNATAddress(t *testing.T) {
	if msg := ValidateNotifyURL("http://100.64.0.1/notify"); msg == "" {
		t.Fatal("ValidateNotifyURL accepted CGNAT notify_url")
	}
}

func TestValidateNotifyURLRejectsUnsupportedScheme(t *testing.T) {
	if msg := ValidateNotifyURL("gopher://pay.example.com/notify"); msg == "" {
		t.Fatal("ValidateNotifyURL accepted unsupported notify_url scheme")
	}
}
