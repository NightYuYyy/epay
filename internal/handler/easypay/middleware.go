package easypay

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Private IP ranges to block for SSRF prevention.
var privateCIDRs = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

var privateNetworks []*net.IPNet

func init() {
	for _, cidr := range privateCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid CIDR %q: %v", cidr, err))
		}
		privateNetworks = append(privateNetworks, n)
	}
}

// ValidateNotifyURL checks the notify_url for two security requirements:
//  1. Must not contain query parameters (protocol requirement)
//  2. Must not point to a private/internal IP address (SSRF prevention)
//
// Returns a user-facing error message string, or empty string if valid.
func ValidateNotifyURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "notify_url格式无效"
	}

	// Reject URLs with query parameters
	if u.RawQuery != "" {
		return "notify_url不能包含查询参数"
	}

	// Extract host (strip port for IP resolution)
	host := u.Hostname()
	if host == "" {
		return "notify_url格式无效"
	}

	// Check if host is a literal private IP
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return "notify_url不允许指向内网地址"
		}
		return ""
	}

	// DNS resolution: resolve hostname and check all returned IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		// If DNS resolution fails, allow the URL (the upstream will fail too).
		// We don't want to block legitimate URLs just because of transient DNS issues.
		return ""
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return "notify_url不允许指向内网地址"
		}
	}

	return ""
}

// isPrivateIP checks whether the given IP is in any of the private CIDR ranges.
func isPrivateIP(ip net.IP) bool {
	for _, n := range privateNetworks {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// sanitizeURL strips leading/trailing whitespace from a URL string.
func sanitizeURL(s string) string {
	return strings.TrimSpace(s)
}
