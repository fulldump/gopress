package safeurl

import (
	"fmt"
	"net"
)

var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // RFC3927 link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Errorf("parse error on %q: %v", cidr, err))
		}
		privateIPBlocks = append(privateIPBlocks, block)
	}
}

func checkPrivateIP(ip net.IP) error {

	if ip.IsLoopback() {
		return fmt.Errorf("IsLoopback")
	}

	if ip.IsLinkLocalUnicast() {
		return fmt.Errorf("IsLinkLocalUnicast")
	}

	if ip.IsLinkLocalMulticast() {
		return fmt.Errorf("IsLinkLocalMulticast")
	}

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return fmt.Errorf("IsPrivateBlock:%s", block.String())
		}
	}

	return nil
}
