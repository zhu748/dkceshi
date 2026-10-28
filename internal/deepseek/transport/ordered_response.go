package transport

import (
	"crypto/tls"
	"io"
	"net"
	"sync"

	"ds2api/internal/config"
	utls "github.com/refraction-networking/utls"
)

func negotiatedProtocol(conn net.Conn) (string, bool) {
	switch secure := conn.(type) {
	case *tls.Conn:
		return secure.ConnectionState().NegotiatedProtocol, true
	case *utls.UConn:
		return secure.ConnectionState().NegotiatedProtocol, true
	default:
		return "", false
	}
}

func closeTransportConn(conn net.Conn) {
	if err := conn.Close(); err != nil {
		config.Logger.Debug("[transport] close connection", "error", err)
	}
}

type orderedResponseBody struct {
	io.ReadCloser
	cleanup func()
	once    sync.Once
}

func (b *orderedResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.once.Do(b.cleanup)
	}
	return n, err
}

func (b *orderedResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.cleanup)
	return err
}
