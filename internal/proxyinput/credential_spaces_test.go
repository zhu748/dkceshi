package proxyinput

import "testing"

func TestPipeImportPreservesLiteralCredentialSpaces(t *testing.T) {
	rows, err := Preview("relay.example|1080| user name | pass word \r\nrelay2.example|1080||", "socks5h", nil)
	if err != nil || len(rows) != 2 {
		t.Fatalf("preview: %v %#v", err, rows)
	}
	if rows[0].Status != "ready" || rows[0].Proxy.Username != " user name " || rows[0].Proxy.Password != " pass word " {
		t.Fatalf("credentials changed: %#v", rows[0])
	}
	if rows[1].Status != "ready" || rows[1].Proxy.Username != "" || rows[1].Proxy.Password != "" {
		t.Fatal("empty credentials lost")
	}
	if _, err := Parse("relay.example|1080|u|p|extra", "socks5"); err == nil {
		t.Fatal("extra fields accepted")
	}
}
