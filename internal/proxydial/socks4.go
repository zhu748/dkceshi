package proxydial

import (
	"context"
	"ds2api/internal/config"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

func socks4(ctx context.Context, d *net.Dialer, p config.Proxy, network, target string) (result net.Conn, resultErr error) {
	if network != "tcp" && network != "tcp4" {
		return nil, fmt.Errorf("SOCKS4/4A 仅支持 IPv4 TCP")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	defer func() {
		resultErr = contextFailure(ctx, resultErr)
	}()
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("目标端口无效")
	}
	ip := net.ParseIP(host)
	remote := p.Type == "socks4a" && ip == nil
	if !remote && ip == nil {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("SOCKS4 目标无 IPv4 地址")
		}
		ip = ips[0]
	}
	if !remote && ip.To4() == nil {
		return nil, fmt.Errorf("SOCKS4/4A 不支持 IPv6 目标")
	}
	request := []byte{4, 1, 0, 0, 0, 0, 0, 1}
	binary.BigEndian.PutUint16(request[2:4], uint16(port))
	if !remote {
		copy(request[4:8], ip.To4())
	}
	request = append(request, []byte(p.Username)...)
	request = append(request, 0)
	if remote {
		request = append(request, []byte(host)...)
		request = append(request, 0)
	}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			closeConn(conn)
		}
	}()
	stop := context.AfterFunc(ctx, func() { closeConn(conn) })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err = conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	for len(request) > 0 {
		n, err := conn.Write(request)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrShortWrite
		}
		request = request[n:]
	}
	var reply [8]byte
	if _, err = io.ReadFull(conn, reply[:]); err != nil {
		return nil, err
	}
	if reply[0] != 0 || reply[1] != 90 {
		return nil, fmt.Errorf("SOCKS4 代理拒绝连接（状态 %d）", reply[1])
	}
	if !stop() || ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	success = true
	return conn, nil
}
