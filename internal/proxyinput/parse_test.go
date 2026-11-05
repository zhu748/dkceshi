package proxyinput

import (
	"ds2api/internal/config"
	"strings"
	"testing"
)

func TestSupplierFormatsAndSelectedProtocol(t *testing.T) {
	inputs := []string{"relay.example:443:user:pass", "user:pass:relay.example:443", "user:pass@relay.example:443", "relay.example:443@user:pass", "relay.example|443|user|pass", "relay.example 443 user pass", "sstp://dXNlcjpwYXNz@relay.example:443", "sstp://user:pass@relay.example:443"}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			p, e := Parse(input, "sstp")
			if e != nil {
				t.Fatal(e)
			}
			if p.Type != "sstp" || p.Host != "relay.example" || p.Port != 443 || p.Username != "user" || p.Password != "pass" {
				t.Fatalf("wrong result: %#v", p)
			}
		})
	}
	for _, kind := range config.ProxyTypes {
		input := "relay.example:5349"
		if kind == "sstp" {
			input = "u:p@relay.example:5349"
		}
		p, e := Parse(input, kind)
		if e != nil {
			t.Fatalf("%s: %v", kind, e)
		}
		if p.Type != kind || p.Port != 5349 {
			t.Fatalf("port changed selected protocol %s", kind)
		}
	}
	p, e := Parse("turns://u:p@relay.example", "sstp")
	if e != nil || p.Type != "turns" || p.Port != 5349 {
		t.Fatalf("explicit prefix lost: %#v %v", p, e)
	}
}
func TestCredentialsAndIPv6(t *testing.T) {
	tests := []struct{ input, user, pass string }{
		{"turn://u:p%40ss%3Aword%23%26%2520@relay.example:3478", "u", "p@ss:word#&%20"},
		{"turn:relay.example:3478?username=u&password=p%2B%26%40", "u", "p+&@"},
		{"relay.example|3478|u%20|p%40@:#", "u%20", "p%40@:#"},
		{"relay.example:3478:u:p@ss:#", "u", "p@ss:#"},
		{"u:p@[2001:db8::1]:3478", "u", "p"},
		{"u:p:[2001:db8::1]:3478", "u", "p"},
		{"turn://IHVzZXIgOiBwYXNzIA==@relay.example:3478", " user ", " pass "},
	}
	for _, tc := range tests {
		p, e := Parse(tc.input, "turn")
		if e != nil {
			t.Fatalf("%s: %v", tc.input, e)
		}
		if p.Username != tc.user || p.Password != tc.pass {
			t.Fatalf("credentials altered for %s: %q %q", tc.input, p.Username, p.Password)
		}
	}
}
func TestRejectsInvalidWithoutEchoingSecrets(t *testing.T) {
	for _, input := range []string{"turn:relay.example:3478?transport=udp", "turn://u:private@relay.example:0", "turn://u:private@relay.example:3478?realm=a&realm=b", "turn://relay.example?insecure=false", "socks4://u:private@relay.example:1080", "sstp://u@relay.example:443", "ftp://u:private@relay.example:21", "turn://u:private%GG@relay.example:3478", "turn://relay.example?password=private&pass=x", "turn://relay.example?unknown=private"} {
		_, e := Parse(input, "socks5")
		if e == nil {
			t.Fatalf("accepted invalid %s", input)
		}
		if strings.Contains(e.Error(), "private") {
			t.Fatalf("error contains secret: %v", e)
		}
	}
}
func TestMixedPreviewPartialSuccessAndDedup(t *testing.T) {
	existing, _ := Parse("turn://u:p@relay.example:3478", "turn")
	text := "relay.example:3478:u:p\nturns://u:p@secure.example:5349\nturn:bad.example?transport=udp\nturns://u:p@secure.example:5349\nsstp://u:p@vpn.example:443\nrelay.example:3478:u:changed"
	rows, e := Preview(text, "turn", []config.Proxy{existing})
	if e != nil {
		t.Fatal(e)
	}
	want := []string{"duplicate", "ready", "invalid", "duplicate", "ready", "ready"}
	if len(rows) != len(want) {
		t.Fatalf("rows=%#v", rows)
	}
	for i, r := range rows {
		if r.Status != want[i] {
			t.Fatalf("row %d: %#v", i, r)
		}
	}
	rows, e = Preview(`["[2001:db8::1]:3478", {"type":"sstp","server":"vpn.example","server_port":443,"username":"u","password":"p","options":{"sni":"cert.example","mtu":1280}}]`, "turn", nil)
	if e != nil || len(rows) != 2 || rows[0].Status != "ready" || rows[1].Status != "ready" {
		t.Fatalf("JSON rows %#v %v", rows, e)
	}
	rows, e = Preview(`[2001:db8::1]:3478`, "turn", nil)
	if e != nil || len(rows) != 1 || rows[0].Status != "ready" {
		t.Fatalf("IPv6 preview %#v %v", rows, e)
	}
}
func TestICEAndStrictJSON(t *testing.T) {
	rows, e := Preview(`{"iceServers":[{"urls":"stun:discover.example"},{"urls":["turn:relay.example:3478?transport=tcp","turn:bad.example?transport=udp","turns:secure.example:5349"],"username":"u","credential":"p@:%20"}]}`, "sstp", nil)
	if e != nil || len(rows) != 4 {
		t.Fatalf("%#v %v", rows, e)
	}
	for i, want := range []string{"skipped", "ready", "invalid", "ready"} {
		if rows[i].Status != want {
			t.Fatalf("row %d=%#v", i, rows[i])
		}
	}
	if rows[1].Proxy.Password != "p@:%20" {
		t.Fatal("ICE credential decoded")
	}
	if _, e = Preview(`{"host":"one","host":"two"}`, "turn", nil); e == nil {
		t.Fatal("duplicate JSON field accepted")
	}
	if _, e = Preview(strings.Repeat("relay:1080\n", 201), "socks5", nil); e == nil {
		t.Fatal("item limit missing")
	}
	if _, e = Preview(strings.Repeat("x", MaxBytes+1), "socks5", nil); e == nil {
		t.Fatal("size limit missing")
	}
}
