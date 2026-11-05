package config

import "testing"

func TestProxyIdentityNormalizesIPv6WithoutChangingLegacyIDs(t *testing.T) {
	p := Proxy{Type: "turn", Host: "2001:db8:0:0::1", Port: 3478}
	old := StableProxyID(p)
	q := p
	q.Host = "2001:db8::1"
	if ProxyIdentity(p) != ProxyIdentity(q) {
		t.Fatal("equivalent IPv6 nodes have different connection identities")
	}
	if NormalizeProxy(p).ID != old {
		t.Fatal("canonical identity changed a legacy generated ID")
	}
	for _, host := range []string{"[relay.example]", "[[::1]]", "relay.example]"} {
		p.Host = host
		if err := ValidateProxyConfig([]Proxy{p}); err == nil {
			t.Fatalf("invalid brackets accepted: %s", host)
		}
	}
}
func TestProxySnapshotDoesNotExposeMutableStoreSlice(t *testing.T) {
	s := &Store{cfg: Config{Proxies: []Proxy{{ID: "one", Type: "turn", Host: "relay.example", Port: 3478}}}}
	proxies := s.ProxySnapshot()
	proxies[0].Host = "changed"
	if s.ProxySnapshot()[0].Host != "relay.example" {
		t.Fatal("proxy snapshot aliases the stored slice")
	}
}
