package main

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

func serveSOCKS(ctx context.Context, listener net.Listener, dial dialFunc) error {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); closeBridge(listener); workers.Wait() }()
	stop := context.AfterFunc(ctx, func() { closeBridge(listener) })
	defer stop()
	slots := make(chan struct{}, 128)
	// Refusals run on their own bounded pool: answering inline in the
	// accept loop let one slow client stall the loop (and thus every
	// other incoming connection) for the full 5-second refusal deadline.
	// Beyond the pool, saturated connections are dropped without a reply.
	refusals := make(chan struct{}, 64)
	var refusers sync.WaitGroup
	defer refusers.Wait()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errors.New("accept_failed")
		}
		select {
		case slots <- struct{}{}:
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-slots }()
				defer closeBridge(conn)
				stop := context.AfterFunc(ctx, func() { closeBridge(conn) })
				defer stop()
				handleSOCKS(ctx, conn, dial)
			}()
		default:
			// Overloaded: finish enough of the SOCKS exchange to
			// reply with a real failure code instead of a bare
			// reset, so the Python side can tell proxy saturation
			// apart from a network error and fail over correctly.
			select {
			case refusals <- struct{}{}:
				refusers.Add(1)
				go func() {
					defer refusers.Done()
					defer func() { <-refusals }()
					defer closeBridge(conn)
					refuseOverloaded(conn)
				}()
			default:
				closeBridge(conn)
			}
		}
	}
}

func socksReply(conn net.Conn, code byte) error {
	_, err := conn.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
	return err
}

// refuseOverloaded answers a CONNECT with "general SOCKS server failure"
// when the bridge is at its concurrency limit. Best effort: any parse
// error just drops the connection.
func refuseOverloaded(conn net.Conn) {
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 5 || header[1] == 0 {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil {
		return
	}
	socksReply(conn, 1)
	diagnostic("socks_overloaded")
}

func handleSOCKS(ctx context.Context, conn net.Conn, dial dialFunc) {
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 5 || header[1] == 0 {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	found := false
	for _, method := range methods {
		found = found || method == 0
	}
	if !found {
		conn.Write([]byte{5, 255})
		return
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil || request[0] != 5 || request[2] != 0 {
		return
	}
	if request[1] != 1 {
		socksReply(conn, 7) // CONNECT only: no BIND or UDP ASSOCIATE.
		return
	}
	var host string
	switch request[3] {
	case 1, 4:
		size := 4
		if request[3] == 4 {
			size = 16
		}
		addr := make([]byte, size)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil || length[0] == 0 {
			return
		}
		addr := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
		host = string(addr)
	default:
		socksReply(conn, 8)
		return
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(conn, port); err != nil {
		return
	}
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	upstream, err := dial(dialCtx, net.JoinHostPort(host, strconv.Itoa(int(port[0])<<8|int(port[1]))))
	cancel()
	if err != nil {
		diagnostic(err.Error())
		socksReply(conn, 1)
		return
	}
	defer closeBridge(upstream)
	stop := context.AfterFunc(ctx, func() { closeBridge(upstream) })
	defer stop()
	if err := socksReply(conn, 0); err != nil {
		return
	}
	conn.SetDeadline(time.Time{})
	upstream.SetDeadline(time.Time{})
	relay(conn, upstream)
}

func relay(a, b net.Conn) {
	done := make(chan error, 2)
	copyTo := func(dst, src net.Conn) {
		_, err := io.Copy(dst, src)
		if closer, ok := dst.(interface{ CloseWrite() error }); ok {
			if closeErr := closer.CloseWrite(); err == nil {
				err = closeErr
			}
		} else {
			closeBridge(dst)
		}
		done <- err
	}
	go copyTo(a, b)
	go copyTo(b, a)
	if err := <-done; err != nil {
		closeBridge(a)
		closeBridge(b)
	}
	<-done
}
