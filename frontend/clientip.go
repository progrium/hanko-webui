package frontend

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// Proxies are reverse proxies (server.frontend.trusted_proxies) whose X-Forwarded-For is believed.
type Proxies []*net.IPNet

// ParseProxies parses CIDRs or bare IPs.
func ParseProxies(entries []string) (Proxies, error) {
	var out Proxies
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if !strings.Contains(e, "/") {
			if ip := net.ParseIP(e); ip != nil {
				bits := 128
				if ip.To4() != nil {
					bits = 32
				}
				e = fmt.Sprintf("%s/%d", e, bits)
			}
		}
		_, n, err := net.ParseCIDR(e)
		if err != nil {
			return nil, fmt.Errorf("%q is not an IP or CIDR", e)
		}
		out = append(out, n)
	}
	return out, nil
}

func (p Proxies) trusted(ip string) bool {
	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}
	for _, n := range p {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

// ClientIP is the TCP peer or, when the peer is a trusted proxy, the right-most X-Forwarded-For
// address that isn't itself a trusted proxy. Returns "" if no valid IP can be determined.
func (p Proxies) ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if p.trusted(host) {
		hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			h := strings.TrimSpace(hops[i])
			if h == "" {
				continue
			}
			host = h
			if !p.trusted(h) {
				break
			}
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}
