package proxydial

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"
)

type hostLookupFunc func(context.Context, string, string) ([]string, error)

var defaultHostLookup hostLookupFunc = func(ctx context.Context, _ string, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

// Local DNS must try the remaining addresses when the first family or route fails.
func dialSOCKS5(ctx context.Context, kind, network, target string, lookup hostLookupFunc, dial DialFunc) (net.Conn, error) {
	if kind != "socks5" && kind != "socks4" {
		return dial(ctx, network, target)
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return dial(ctx, network, target)
	}
	if lookup == nil {
		lookup = defaultHostLookup
	}
	query := "ip"
	if kind == "socks4" {
		query = "ip4"
		network = "tcp4"
	}
	addresses, err := lookup(ctx, query, host)
	if err != nil {
		return nil, err
	}
	candidates := []string{}
	seen := map[string]bool{}
	for _, value := range addresses {
		ip, err := netip.ParseAddr(value)
		if err != nil {
			continue
		}
		ip = ip.Unmap()
		if (network == "tcp4" && !ip.Is4()) || (network == "tcp6" && ip.Is4()) {
			continue
		}
		endpoint := net.JoinHostPort(ip.String(), port)
		if !seen[endpoint] {
			seen[endpoint] = true
			candidates = append(candidates, endpoint)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("目标域名未返回可用 IP 地址")
	}
	for i, address := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		attemptCtx := ctx
		cancel := func() {}
		if deadline, ok := ctx.Deadline(); ok && len(candidates)-i > 1 {
			attemptCtx, cancel = context.WithTimeout(ctx, time.Until(deadline)/time.Duration(len(candidates)-i))
		}
		conn, dialErr := dial(attemptCtx, network, address)
		cancel()
		if dialErr == nil {
			return conn, nil
		}
		err = dialErr
	}
	return nil, err
}
