package main

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"
)

func dialResolved(ctx context.Context, target string, ipv4Only bool, dial func(context.Context, *net.TCPAddr) (net.Conn, error)) (net.Conn, error) {
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return nil, errors.New("invalid_target")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid_target")
	}
	// Must not use net.DefaultResolver: yuhaiin's netapi package replaces it
	// with an uninitialized bootstrap resolver (see dns.go).
	ips, err := systemResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, errors.New("target_dns_failed")
	}
	// Prefer IPv4 for common IPv4-only relays and SSTP IPCP networks.
	var ordered []net.IPAddr
	for _, ipv4 := range []bool{true, false} {
		for _, ip := range ips {
			if (ip.IP.To4() != nil) == ipv4 && (!ipv4Only || ipv4) {
				ordered = append(ordered, ip)
			}
		}
	}
	if len(ordered) == 0 {
		// DNS succeeded but returned no usable A/AAAA record: report the
		// resolution failure instead of the misleading family error.
		return nil, errors.New("target_dns_failed")
	}
	err = errors.New("target_address_family_unsupported")
	for i, ip := range ordered {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Spread the remaining dial budget across the remaining
		// candidates so one black-holed address cannot starve the
		// rest; a single candidate keeps the whole budget.
		dialCtx := ctx
		var cancelDial context.CancelFunc
		if deadline, ok := ctx.Deadline(); ok && len(ordered)-i > 1 {
			if per := time.Until(deadline) / time.Duration(len(ordered)-i); per > 0 {
				dialCtx, cancelDial = context.WithTimeout(ctx, per)
			}
		}
		var conn net.Conn
		conn, err = dial(dialCtx, &net.TCPAddr{IP: ip.IP, Zone: ip.Zone, Port: port})
		if cancelDial != nil {
			cancelDial()
		}
		if err == nil {
			return conn, nil
		}
	}
	return nil, err
}
