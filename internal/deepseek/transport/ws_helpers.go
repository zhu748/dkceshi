package transport

import (
	"bufio"
	"crypto/sha1"
	"io"
	"net"
)

func sha1Sum(value string) []byte {
	sum := sha1.Sum([]byte(value))
	return sum[:]
}

func writeAll(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}

// NewWSConnForTest constructs a minimal WSConn wrapping conn for internal test suites.
func NewWSConnForTest(conn net.Conn) *WSConn {
	return &WSConn{conn: conn, br: bufio.NewReaderSize(conn, 64*1024)}
}
