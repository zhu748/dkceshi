package proxydial

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
)

const maxProxyHeaders = 64 << 10

var errProxyHeadersTooLarge = errors.New("代理 CONNECT 响应头超过 64 KiB")

type headerLimitReader struct {
	reader    io.Reader
	remaining int
	unlimited bool
}

func (r *headerLimitReader) Read(p []byte) (int, error) {
	if !r.unlimited {
		if r.remaining == 0 {
			return 0, errProxyHeadersTooLarge
		}
		if len(p) > r.remaining {
			p = p[:r.remaining]
		}
	}
	n, err := r.reader.Read(p)
	if !r.unlimited {
		r.remaining -= n
	}
	return n, err
}
func readConnectResponse(conn net.Conn, req *http.Request) (*bufio.Reader, error) {
	limited := &headerLimitReader{reader: conn, remaining: maxProxyHeaders}
	reader := bufio.NewReader(limited)
	for i := 0; i < 6; i++ {
		resp, err := http.ReadResponse(reader, req)
		if err != nil {
			if errors.Is(err, errProxyHeadersTooLarge) {
				return nil, errProxyHeadersTooLarge
			}
			return nil, fmt.Errorf("代理 CONNECT 响应无效")
		}
		if resp.StatusCode >= 100 && resp.StatusCode < 200 && resp.StatusCode != 101 {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("代理 CONNECT 返回 HTTP %d", resp.StatusCode)
		}
		// Only handshake headers are limited; the buffered tunnel keeps every byte.
		limited.unlimited = true
		return reader, nil
	}
	return nil, fmt.Errorf("代理 CONNECT 返回过多临时响应")
}
