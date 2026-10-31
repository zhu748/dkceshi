package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	fh2 "github.com/bogdanfinn/fhttp/http2"
)

type h2Session struct {
	conn   net.Conn
	client *fh2.ClientConn
	timer  *time.Timer
}

func h2IdentityKey(req *http.Request, headers []OrderedHeader) string {
	identity := []string{req.URL.Scheme, req.URL.Host, req.Host}
	for _, name := range []string{"authorization", "x-device-id", "x-rangers-id", "x-device-model", "user-agent"} {
		value := ""
		for _, header := range headers {
			if strings.EqualFold(header.Name, name) {
				value = header.Value
				break
			}
		}
		identity = append(identity, value)
	}
	sum := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
	return hex.EncodeToString(sum[:])
}

// Sessions are checked out exclusively. Cancelling one request can close its
// connection without affecting another account or an active sibling stream.
func (c *H2FramedClient) takeIdle(key string) (*h2Session, uint64) {
	c.poolMu.Lock()
	defer c.poolMu.Unlock()
	for len(c.idle[key]) > 0 {
		list := c.idle[key]
		session := list[len(list)-1]
		c.idle[key] = list[:len(list)-1]
		if len(c.idle[key]) == 0 {
			delete(c.idle, key)
		}
		c.idleCount--
		session.timer.Stop()
		if session.client.CanTakeNewRequest() {
			return session, c.poolGeneration
		}
		closeTransportConn(session.conn)
	}
	return nil, c.poolGeneration
}

func (c *H2FramedClient) returnIdle(key string, session *h2Session, generation uint64, reusable bool) {
	c.poolMu.Lock()
	defer c.poolMu.Unlock()
	if !reusable || generation != c.poolGeneration || c.idleCount >= 64 || len(c.idle[key]) >= 2 || !session.client.CanTakeNewRequest() {
		closeTransportConn(session.conn)
		return
	}
	if c.idle == nil {
		c.idle = make(map[string][]*h2Session)
	}
	session.timer = time.AfterFunc(90*time.Second, func() { c.expireIdle(key, session) })
	c.idle[key] = append(c.idle[key], session)
	c.idleCount++
}

func (c *H2FramedClient) expireIdle(key string, session *h2Session) {
	c.poolMu.Lock()
	defer c.poolMu.Unlock()
	for i, candidate := range c.idle[key] {
		if candidate == session {
			c.idle[key] = append(c.idle[key][:i], c.idle[key][i+1:]...)
			if len(c.idle[key]) == 0 {
				delete(c.idle, key)
			}
			c.idleCount--
			closeTransportConn(session.conn)
			return
		}
	}
}

func (c *H2FramedClient) CloseIdleConnections() {
	c.poolMu.Lock()
	c.poolGeneration++
	idle := c.idle
	c.idle = nil
	c.idleCount = 0
	for _, sessions := range idle {
		for _, session := range sessions {
			session.timer.Stop()
			closeTransportConn(session.conn)
		}
	}
	c.poolMu.Unlock()
}
