package transport

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

// writeServerFrame writes an unmasked frame, as a WebSocket server does.
func writeServerFrame(conn net.Conn, fin bool, opcode byte, payload []byte) error {
	var header [10]byte
	if fin {
		header[0] = 0x80 | opcode
	} else {
		header[0] = opcode
	}
	n := 2
	switch {
	case len(payload) < 126:
		header[1] = byte(len(payload))
	case len(payload) <= 0xffff:
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:], uint16(len(payload)))
		n = 4
	default:
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(len(payload)))
		n = 10
	}
	if err := writeAll(conn, header[:n]); err != nil {
		return err
	}
	return writeAll(conn, payload)
}

// readClientFrame parses a masked client frame.
func readClientFrame(conn net.Conn) (byte, []byte, bool, error) {
	var header [2]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return 0, nil, false, err
	}
	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(conn, ext[:]); err != nil {
			return 0, nil, false, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(conn, ext[:]); err != nil {
			return 0, nil, false, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(conn, mask[:]); err != nil {
			return 0, nil, false, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return 0, nil, false, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return opcode, payload, masked, nil
}

func newPipeWS(t *testing.T) (*WSConn, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		closeTransportConn(client)
		closeTransportConn(server)
	})
	return &WSConn{conn: client, br: bufio.NewReader(client)}, server
}

func TestWSConnReadTextAndBinary(t *testing.T) {
	ws, server := newPipeWS(t)
	go func() {
		_ = writeServerFrame(server, true, WSOpText, []byte("hello"))
		_ = writeServerFrame(server, true, WSOpBinary, []byte{1, 2, 3})
	}()

	opcode, payload, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read text: %v", err)
	}
	if opcode != WSOpText || string(payload) != "hello" {
		t.Fatalf("text opcode=%#x payload=%q", opcode, payload)
	}

	opcode, payload, err = ws.ReadMessage()
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if opcode != WSOpBinary || len(payload) != 3 || payload[0] != 1 || payload[2] != 3 {
		t.Fatalf("binary opcode=%#x payload=%v", opcode, payload)
	}
}

func TestWSConnReadFragmentedMessage(t *testing.T) {
	ws, server := newPipeWS(t)
	go func() {
		_ = writeServerFrame(server, false, WSOpText, []byte("he"))
		_ = writeServerFrame(server, false, WSOpContinuation, []byte("ll"))
		_ = writeServerFrame(server, true, WSOpContinuation, []byte("o"))
	}()

	opcode, payload, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read fragmented: %v", err)
	}
	if opcode != WSOpText || string(payload) != "hello" {
		t.Fatalf("opcode=%#x payload=%q want text/hello", opcode, payload)
	}
}

func TestWSConnAnswersPing(t *testing.T) {
	ws, server := newPipeWS(t)
	serverErr := make(chan error, 1)
	go func() {
		if err := writeServerFrame(server, true, WSOpPing, []byte("p")); err != nil {
			serverErr <- err
			return
		}
		opcode, payload, masked, err := readClientFrame(server)
		if err != nil {
			serverErr <- err
			return
		}
		if opcode != WSOpPong || string(payload) != "p" || !masked {
			serverErr <- io.ErrUnexpectedEOF
			return
		}
		serverErr <- writeServerFrame(server, true, WSOpText, []byte("after"))
	}()

	if err := ws.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	opcode, payload, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read after ping: %v", err)
	}
	if opcode != WSOpText || string(payload) != "after" {
		t.Fatalf("opcode=%#x payload=%q want text/after", opcode, payload)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestWSConnWriteFrameIsMasked(t *testing.T) {
	ws, server := newPipeWS(t)
	serverErr := make(chan error, 1)
	go func() {
		opcode, payload, masked, err := readClientFrame(server)
		if err != nil {
			serverErr <- err
			return
		}
		if opcode != WSOpText || string(payload) != "hello" || !masked {
			serverErr <- io.ErrUnexpectedEOF
			return
		}
		serverErr <- nil
	}()

	if err := ws.WriteText([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestWSConnCloseSendsCloseFrame(t *testing.T) {
	ws, server := newPipeWS(t)
	serverErr := make(chan error, 1)
	go func() {
		opcode, payload, _, err := readClientFrame(server)
		if err != nil {
			serverErr <- err
			return
		}
		if opcode != WSOpClose || len(payload) < 2 {
			serverErr <- io.ErrUnexpectedEOF
			return
		}
		serverErr <- nil
	}()

	if err := ws.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestWSConnRejectsUnexpectedFramesDuringFragmentation(t *testing.T) {
	ws, server := newPipeWS(t)
	go func() {
		// Start fragment (fin=false)
		_ = writeServerFrame(server, false, WSOpText, []byte("start"))
		// Send another Text frame instead of Continuation (invalid per RFC 6455)
		_ = writeServerFrame(server, true, WSOpText, []byte("invalid"))
	}()

	_, _, err := ws.ReadMessage()
	if err == nil || err.Error() != "websocket: expected continuation frame" {
		t.Fatalf("expected 'expected continuation frame' error, got %v", err)
	}

	ws2, server2 := newPipeWS(t)
	go func() {
		// Send Continuation frame when not in fragment
		_ = writeServerFrame(server2, true, WSOpContinuation, []byte("orphan"))
	}()
	_, _, err = ws2.ReadMessage()
	if err == nil || err.Error() != "websocket: unexpected continuation frame" {
		t.Fatalf("expected 'unexpected continuation frame' error, got %v", err)
	}
}

func TestWSConnCloseDoesNotBlockOnUnreadPeer(t *testing.T) {
	ws, _ := newPipeWS(t)
	started := time.Now()
	if err := ws.Close(); err == nil {
		t.Fatal("expected write timeout from unread peer")
	}
	if time.Since(started) > time.Second {
		t.Fatal("close blocked on peer")
	}
}
