package proxyinput

import (
	"strings"
	"testing"
)

func TestInvalidInputCannotBecomeAnEditableValidNode(t *testing.T) {
	for _, input := range []string{"turn:relay.example:3478?transport=udp", "turns://relay.example:5349?unknown=ignored", "turns://relay.example:5349?sni=valid&mtu=0", "turn://user:%FF@relay.example:3478"} {
		rows, err := Preview(input, "turn", nil)
		if err != nil || len(rows) != 1 {
			t.Fatalf("preview failed: %v", err)
		}
		if rows[0].Status != "invalid" || rows[0].Proxy != nil {
			t.Fatalf("invalid input produced editable fields: %#v", rows[0])
		}
	}
	rows, err := Preview("sstp://vpn.example:443", "sstp", nil)
	if err != nil || len(rows) != 1 || rows[0].Status != "incomplete" || rows[0].Proxy == nil {
		t.Fatalf("SSTP address should allow credential completion: %#v %v", rows, err)
	}
}

func TestJSONProtocolDefaultsAndStrictAdvancedTypes(t *testing.T) {
	rows, err := Preview(`{"type":" SSTP ","host":"vpn.example","username":"u","password":"p"}`, "http", nil)
	if err != nil || len(rows) != 1 || rows[0].Status != "ready" || rows[0].Proxy.Port != 443 {
		t.Fatalf("protocol whitespace changed default port: %#v %v", rows, err)
	}
	for _, opts := range []string{`{"sni":true}`, `{"ca":123}`, `{"realm":false}`, `{"username":"hidden"}`, `{"mtu":true}`} {
		rows, err := Preview(`{"type":"sstp","host":"vpn.example","username":"u","password":"p","options":`+opts+`}`, "sstp", nil)
		if err != nil || len(rows) != 1 || rows[0].Status != "invalid" {
			t.Fatalf("advanced types accepted %s: %#v %v", opts, rows, err)
		}
	}
}

func TestLineJSONExpansionAndEmptyContainers(t *testing.T) {
	text := "socks5h://relay.example:1080\n" + `{"iceServers":[{"urls":["turn:one.example","turns:two.example"],"username":"u","credential":"p"}]}`
	rows, err := Preview(text, "sstp", nil)
	if err != nil || len(rows) != 3 {
		t.Fatalf("line ICE JSON not expanded: %#v %v", rows, err)
	}
	for _, r := range rows {
		if r.Status != "ready" {
			t.Fatalf("invalid expanded row %#v", r)
		}
	}
	for _, input := range []string{"[]", `{"iceServers":[]}`} {
		if rows, e := Preview(input, "turn", nil); e == nil || len(rows) > 0 {
			t.Fatalf("empty container treated as a node: %#v %v", rows, e)
		}
	}
	rows, err = Preview("{broken json\nturn:valid.example", "turn", nil)
	if err != nil || len(rows) != 2 || rows[0].Status != "invalid" || rows[1].Status != "ready" {
		t.Fatalf("bad JSON line discarded valid sibling: %#v %v", rows, err)
	}
}

func TestURLAuthorityBeatsSupplierHeuristics(t *testing.T) {
	p, err := Parse("socks5://user:1234:extra@relay.example:1080?username=ignored", "socks5")
	if err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("duplicate user was ignored: %#v %v", p, err)
	}
	p, err = Parse("turns://user:password@relay.example:5349?sni=cert.example#备用", "turn")
	if err != nil || p.ServerName != "cert.example" || p.Name != "备用" {
		t.Fatalf("URL metadata lost: %#v %v", p, err)
	}
}
