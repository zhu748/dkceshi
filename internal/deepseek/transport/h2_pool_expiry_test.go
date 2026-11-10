package transport

import (
	"io"
	"net/http"
	"testing"
)

func TestIdleExpiryCannotRetireAReusedSession(t *testing.T) {
	client, url, dials := pooledTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, "ok"); err != nil {
			t.Error(err)
		}
	})
	pooledGet(t, client, url, "installation", "token")
	var key string
	var session *h2Session
	var oldVersion uint64
	client.poolMu.Lock()
	for k, sessions := range client.idle {
		key, session = k, sessions[0]
		oldVersion = session.idleVersion
	}
	client.poolMu.Unlock()
	if session == nil {
		t.Fatal("completed response did not return an idle session")
	}
	checkedOut, generation := client.takeIdle(key)
	if checkedOut != session {
		t.Fatal("expected the previously idle session")
	}
	client.returnIdle(key, session, generation, true)
	// Simulate the old timer callback which had already started when Stop
	// was called, but could only acquire poolMu after this new idle stay.
	client.expireIdle(key, session, oldVersion)
	pooledGet(t, client, url, "installation", "token")
	if dials.Load() != 1 {
		t.Fatal("an obsolete timeout closed the reused connection")
	}
	client.poolMu.Lock()
	currentVersion := session.idleVersion
	session.timer.Stop()
	client.poolMu.Unlock()
	client.expireIdle(key, session, currentVersion)
	pooledGet(t, client, url, "installation", "token")
	if dials.Load() != 2 {
		t.Fatal("the current idle timeout did not retire the connection")
	}
}
