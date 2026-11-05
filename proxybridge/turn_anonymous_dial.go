package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	stun "github.com/pion/stun/v4"
	"net"
	"strconv"
	"sync"
	"time"
)

// dialTURNAnonymous implements outbound RFC 6062 relaying without
// credentials: Allocate → Connect(peer) → ConnectionBind on a second
// connection, then plain relayed bytes.
func (c config) dialTURNAnonymous(ctx context.Context, peer *net.TCPAddr, tlsConfig *tls.Config) (net.Conn, error) {
	serverAddr := net.JoinHostPort(c.Server, strconv.Itoa(c.Port))
	control, _, err := dialServer(ctx, serverAddr, tlsConfig)
	if err != nil {
		return nil, err
	}
	complete := false
	stopControl := context.AfterFunc(ctx, func() { closeBridge(control) })
	defer stopControl()
	defer func() {
		if !complete {
			closeBridge(control)
		}
	}()
	clearControl, err := withTimeout(ctx, control)
	if err != nil {
		return nil, err
	}
	defer clearControl()

	allocate := allocateRequest(peer.IP.To4() != nil)
	response, err := rawSTUNTransaction(control, allocate)
	if err != nil {
		return nil, err
	}
	if response.Type.Class != stun.ClassSuccessResponse {
		return nil, fmt.Errorf("allocate: %w", stunError(response))
	}
	lifetime := 600 * time.Second
	if value, err := response.Get(stun.AttrLifetime); err == nil && len(value) == 4 {
		if seconds := binary.BigEndian.Uint32(value); seconds > 0 {
			lifetime = time.Duration(seconds) * time.Second
		}
	}

	// CreatePermission for the peer before Connect, matching the Pion client
	// and the edgetunnel Worker implementation: some servers reject a Connect
	// for a peer without an installed permission.
	permission := stun.MustBuild(
		stun.TransactionID,
		stun.NewType(stun.MethodCreatePermission, stun.ClassRequest),
		peerAddrSetter{IP: peer.IP, Port: peer.Port},
		stun.Fingerprint,
	)
	response, err = rawSTUNTransaction(control, permission)
	if err != nil {
		return nil, err
	}
	if response.Type.Class != stun.ClassSuccessResponse {
		return nil, fmt.Errorf("create permission: %w", stunError(response))
	}

	connect := stun.MustBuild(
		stun.TransactionID,
		stun.NewType(stun.MethodConnect, stun.ClassRequest),
		peerAddrSetter{IP: peer.IP, Port: peer.Port},
		stun.Fingerprint,
	)
	response, err = rawSTUNTransaction(control, connect)
	if err != nil {
		return nil, err
	}
	if response.Type.Class != stun.ClassSuccessResponse {
		return nil, fmt.Errorf("connect: %w", stunError(response))
	}
	cid, err := response.Get(stun.AttrConnectionID)
	if err != nil || len(cid) != 4 {
		return nil, errors.New("connect: missing CONNECTION-ID")
	}

	// RFC 6062 requires the same server transport address for both
	// connections, exactly like the Pion path.
	data, tcpData, err := dialServer(ctx, control.RemoteAddr().String(), tlsConfig)
	if err != nil {
		return nil, err
	}
	stopData := context.AfterFunc(ctx, func() { closeBridge(data) })
	defer stopData()
	// Keep a floor on the binding exchange even when the caller passes a
	// context without a deadline: a silent server would otherwise block
	// this dial forever instead of failing within seconds.
	bindDeadline := time.Now().Add(10 * time.Second)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(bindDeadline) {
		bindDeadline = deadline
	}
	data.SetDeadline(bindDeadline)
	adapter := &turnDataConn{TCPConn: tcpData, stream: data, binding: true}
	bind := stun.MustBuild(
		stun.TransactionID,
		stun.NewType(stun.MethodConnectionBind, stun.ClassRequest),
		stun.RawAttribute{Type: stun.AttrConnectionID, Value: cid},
		stun.Fingerprint,
	)
	if _, err := adapter.Write(bind.Raw); err != nil {
		closeBridge(data)
		return nil, err
	}
	head := make([]byte, stunHeaderSize)
	if _, err := adapter.Read(head); err != nil || !stun.IsMessage(head) {
		closeBridge(data)
		return nil, errors.New("connection bind: invalid response")
	}
	body := make([]byte, binary.BigEndian.Uint16(head[2:4]))
	if _, err := adapter.Read(body); err != nil {
		closeBridge(data)
		return nil, err
	}
	bindResponse := &stun.Message{Raw: append(head, body...)}
	if err := bindResponse.Decode(); err != nil {
		closeBridge(data)
		return nil, err
	}
	if bindResponse.Type.Class != stun.ClassSuccessResponse {
		closeBridge(data)
		return nil, fmt.Errorf("connection bind: %w", stunError(bindResponse))
	}
	adapter.binding = false
	data.SetDeadline(time.Time{})
	if ctx.Err() != nil || !stopControl() || !stopData() {
		closeBridge(data)
		return nil, errors.New("turn_dial_cancelled")
	}
	complete = true

	// Single ownership: rawAllocation.stop() is the ONLY closer of the
	// closed channel (and of control). kill() delegates there and closes
	// the data connection exactly once. Previously kill guarded the same
	// channel with a second sync.Once, so the refresh-failure path
	// (stop followed by kill) panicked with "close of closed channel"
	// and crashed the whole bridge process.
	allocation := &rawAllocation{control: control, lifetime: lifetime, closed: make(chan struct{})}
	var dataOnce sync.Once
	kill := func() {
		allocation.stop()
		dataOnce.Do(func() { closeBridge(data) })
	}
	allocation.startRefresh(kill)
	// The dial context is cancelled by handleSOCKS right after this returns
	// (same as the Pion path): the connection must survive it, ownership is
	// purely cleanup-driven from here on.
	return &managedConn{Conn: adapter, cleanup: kill}, nil
}
