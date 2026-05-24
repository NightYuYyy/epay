package easypay

import (
	"fmt"
	"net"
	"net/url"
)

// Private IP ranges to block for SSRF prevention.
var privateCIDRs = []string{
	"0.0.0.0/8",
	"100.64.0.0/10",
	"10.0.0.0/8",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"255.255.255.255/32",
	"::/128",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
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
	if u.Scheme != "http" && u.Scheme != "https" {
		return "notify_url仅支持http或https"
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
		return "notify_url域名无法解析"
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
