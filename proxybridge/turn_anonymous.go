package main

// Anonymous-allocation TURN support (RFC 6062 over raw STUN).
//
// The Pion TURN client always expects the RFC 5766 long-term-credential
// dance: it treats the first Allocate response as a 401 challenge and
// unconditionally reads NONCE/REALM from it (client.go sendAllocateRequest).
// Open TURN servers that accept unauthenticated allocations reply with a
// bare success response, so Pion fails with an attribute error even though
// the allocation succeeded.
//
// At startup the bridge probes the server with one raw Allocate request.
// Success selects the raw dialer below; an error response (401/438/...)
// selects the Pion client. The raw dialer mirrors Pion's RFC 6062 flow
// without credentials: Allocate, Connect(peer), then a second connection
// running ConnectionBind, followed by application bytes and periodic
// Refresh requests that fail closed.

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	stun "github.com/pion/stun/v4"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

const stunHeaderSize = 20

// peerAddrSetter writes XOR-PEER-ADDRESS (same wire format as
// XORMappedAddress, different attribute type).
type peerAddrSetter stun.XORMappedAddress

func (s peerAddrSetter) AddTo(m *stun.Message) error {
	return stun.XORMappedAddress(s).AddToAs(m, stun.AttrXORPeerAddress)
}

// rawSTUNTransaction writes one request and reads the response carrying the
// same transaction ID. STUN-over-TCP framing: 20-byte header, then the
// announced length.
//
// The control connection is not request/response exclusive: any peer that
// hits the relayed transport address makes the server emit an unsolicited
// ConnectionAttempt indication (RFC 6062 section 6.2.2), and retransmitted
// responses to earlier transactions can still arrive. Reading exactly one
// message used to consume such traffic as the reply, failing the Refresh,
// killing the allocation and desynchronizing the stream. Unsolicited
// messages are therefore read past; the connection deadline bounds the loop.
func rawSTUNTransaction(conn net.Conn, request *stun.Message) (*stun.Message, error) {
	if _, err := conn.Write(request.Raw); err != nil {
		return nil, err
	}
	head := make([]byte, stunHeaderSize)
	for {
		if _, err := io.ReadFull(conn, head); err != nil {
			return nil, err
		}
		if !stun.IsMessage(head) {
			return nil, errors.New("not a STUN message")
		}
		body := make([]byte, binary.BigEndian.Uint16(head[2:4]))
		if _, err := io.ReadFull(conn, body); err != nil {
			return nil, err
		}
		response := &stun.Message{Raw: append(head, body...)}
		if err := response.Decode(); err != nil {
			return nil, err
		}
		if response.Type.Class == stun.ClassIndication {
			continue
		}
		if response.TransactionID != request.TransactionID {
			continue
		}
		return response, nil
	}
}

func withTimeout(ctx context.Context, conn net.Conn) (func(), error) {
	deadline := time.Now().Add(10 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return func() {}, err
	}
	return func() { conn.SetDeadline(time.Time{}) }, nil
}

// requestedFamily encodes REQUESTED-ADDRESS-FAMILY (0x0017): 0x01 = IPv4,
// 0x02 = IPv6, plus three reserved zero bytes.
func requestedFamily(ipv4 bool) stun.RawAttribute {
	value := []byte{2, 0, 0, 0}
	if ipv4 {
		value[0] = 1
	}
	return stun.RawAttribute{Type: stun.AttrRequestedAddressFamily, Value: value}
}

func allocateRequest(ipv4 bool) *stun.Message {
	return stun.MustBuild(
		stun.TransactionID,
		stun.NewType(stun.MethodAllocate, stun.ClassRequest),
		stun.RawAttribute{Type: stun.AttrRequestedTransport, Value: []byte{6, 0, 0, 0}}, // 6 = TCP (RFC 6062)
		requestedFamily(ipv4),
		stun.Fingerprint,
	)
}

func stunError(response *stun.Message) error {
	var code stun.ErrorCodeAttribute
	if err := code.GetFrom(response); err == nil {
		return fmt.Errorf("stun error %d (%s)", code.Code, code.Reason)
	}
	return fmt.Errorf("%s", response.Type)
}

// detectTURNMode probes the server once: a success response to an
// unauthenticated Allocate selects the anonymous dialer; any error response
// (401/438/300/...) selects the Pion client, which handles the standard
// credential flow.
func detectTURNMode(ctx context.Context, c config, tlsConfig *tls.Config) (string, error) {
	// Tight budget: the Python manager marks the bridge as failed when the
	// ready marker does not appear within 10 seconds of launch.
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	control, _, err := dialServer(ctx, net.JoinHostPort(c.Server, strconv.Itoa(c.Port)), tlsConfig)
	if err != nil {
		return "", err
	}
	defer closeBridge(control)
	clear, err := withTimeout(ctx, control)
	if err != nil {
		return "", err
	}
	defer clear()
	// Most open relays are IPv4-only; probe the IPv4 family first and fall
	// back to the Pion client whenever the server answers with an error.
	response, err := rawSTUNTransaction(control, allocateRequest(true))
	if err != nil {
		return "", err
	}
	if response.Type.Class == stun.ClassSuccessResponse {
		// The probe just created a real allocation. Closing the TCP control
		// connection releases it on RFC-compliant servers; an explicit
		// deallocation (RFC 5766 section 7.3: Refresh with LIFETIME 0) also
		// covers servers that keep relayed ports reserved until expiry.
		// Best effort: the mode decision does not depend on it.
		release := stun.MustBuild(
			stun.TransactionID,
			stun.NewType(stun.MethodRefresh, stun.ClassRequest),
			stun.RawAttribute{Type: stun.AttrLifetime, Value: make([]byte, 4)},
			stun.Fingerprint,
		)
		_, _ = rawSTUNTransaction(control, release)
		return "anonymous", nil
	}
	return "pion", nil
}

type rawAllocation struct {
	control  net.Conn
	lifetime time.Duration
	closed   chan struct{}
	once     sync.Once
}

func (a *rawAllocation) stop() {
	a.once.Do(func() { close(a.closed); closeBridge(a.control) })
}

// refreshInterval schedules the next refresh inside the granted window:
// half the granted lifetime with a small floor, and strictly before the
// expiry. The old fixed floor of 5s delayed the first refresh past the
// expiry of allocations granted less than 10 seconds.
func refreshInterval(granted time.Duration) time.Duration {
	if granted <= 0 {
		return 0
	}
	interval := granted / 2
	if interval < 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	if interval >= granted { // sub-second grants
		interval = granted / 3
	}
	if interval <= 0 {
		interval = time.Millisecond
	}
	return interval
}

// startRefresh keeps the allocation alive; a failed refresh fails closed by
// closing the control connection and notifying the data side.
//
// RFC 5766 section 6.2 lets the server grant less time than requested, so
// the next refresh follows the LIFETIME returned by each response. The old
// loop kept the interval derived from the Allocate response: a server that
// reduced the lifetime let the allocation expire between refreshes.
func (a *rawAllocation) startRefresh(kill func()) {
	go func() {
		timer := time.NewTimer(refreshInterval(a.lifetime))
		defer timer.Stop()
		granted := a.lifetime
		for {
			select {
			case <-a.closed:
				return
			case <-timer.C:
			}
			clear, err := withTimeout(context.Background(), a.control)
			if err != nil {
				a.stop()
				kill()
				return
			}
			refresh := stun.MustBuild(
				stun.TransactionID,
				stun.NewType(stun.MethodRefresh, stun.ClassRequest),
				stun.RawAttribute{Type: stun.AttrLifetime,
					Value: binary.BigEndian.AppendUint32(nil, uint32(a.lifetime/time.Second))},
				stun.Fingerprint,
			)
			response, err := rawSTUNTransaction(a.control, refresh)
			clear()
			if err != nil || response.Type.Class != stun.ClassSuccessResponse {
				a.stop()
				kill()
				return
			}
			if value, err := response.Get(stun.AttrLifetime); err == nil && len(value) == 4 {
				granted = time.Duration(binary.BigEndian.Uint32(value)) * time.Second
			}
			if next := refreshInterval(granted); next <= 0 {
				// The server granted a zero lifetime: the allocation is gone.
				a.stop()
				kill()
				return
			} else {
				timer.Reset(next)
			}
		}
	}()
}
