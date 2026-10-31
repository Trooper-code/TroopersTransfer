package relay

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackpal/gateway"
)

// LocalIP returns this PC's address on the home network: the one used to
// reach the internet. No packets are sent; UDP "dial" only picks a route.
func LocalIP() (string, error) {
	c, err := net.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return "", err
	}
	defer c.Close()
	ip := c.LocalAddr().(*net.UDPAddr).IP
	if ip == nil || ip.IsUnspecified() {
		return "", errors.New("no network connection")
	}
	return ip.String(), nil
}

// RouterIP returns the default gateway, which is normally the router's
// settings page address. Empty if it can't be found.
func RouterIP() string {
	gw, err := gateway.DiscoverGateway()
	if err != nil || gw == nil {
		return ""
	}
	return gw.String()
}

// publicIPServices answer a plain GET with just the caller's IPv4 address.
var publicIPServices = []string{
	"https://api.ipify.org",
	"https://ipv4.icanhazip.com",
	"https://ifconfig.me/ip",
}

// PublicIP asks a few well-known services what address the internet sees
// for this connection. It returns the first plausible answer.
func PublicIP(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error = errors.New("no public IP service answered")
	for _, u := range publicIPServices {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		ip := net.ParseIP(strings.TrimSpace(string(b)))
		if ip != nil && ip.To4() != nil {
			return ip.String(), nil
		}
		lastErr = errors.New("unexpected answer from " + u)
	}
	return "", lastErr
}

var cgnatRange = mustCIDR("100.64.0.0/10")

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// IsCGNAT reports an address in the shared range ISPs use for carrier-grade
// NAT (100.64.0.0/10).
func IsCGNAT(ip string) bool {
	p := net.ParseIP(ip)
	return p != nil && cgnatRange.Contains(p)
}

// IsPrivate reports a home-network-only address (10.x, 172.16-31.x,
// 192.168.x).
func IsPrivate(ip string) bool {
	p := net.ParseIP(ip)
	return p != nil && p.IsPrivate()
}
