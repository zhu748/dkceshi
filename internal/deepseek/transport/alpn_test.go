package transport

import (
	"net"
	"testing"

	utls "github.com/refraction-networking/utls"
)

func TestAndroidPresetAddsMissingALPNToWireState(t *testing.T) {
	client, server := net.Pipe()
	defer closeTransportConn(client)
	defer closeTransportConn(server)
	conn := utls.UClient(client, &utls.Config{ServerName: "chat.deepseek.com"}, utls.HelloAndroid_11_OkHttp)
	if err := forceALPN(conn, []string{"h2", "http/1.1"}); err != nil {
		t.Fatal(err)
	}
	if got := conn.HandshakeState.Hello.AlpnProtocols; len(got) != 2 || got[0] != "h2" {
		t.Fatalf("ALPN absent from ClientHello: %v", got)
	}
	if err := forceALPN(conn, []string{"http/1.1"}); err != nil {
		t.Fatal(err)
	}
	if got := conn.HandshakeState.Hello.AlpnProtocols; len(got) != 1 || got[0] != "http/1.1" {
		t.Fatalf("ALPN override not applied: %v", got)
	}
}
