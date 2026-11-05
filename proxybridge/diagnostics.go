package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Only controlled codes cross the parent channel; never copy peer error text.
func diagnosticError(err error) {
	code := "proxy_protocol_failed"
	var protocol protocolError
	if errors.As(err, &protocol) {
		code = protocol.code
	} else {
		value := err.Error()
		for _, known := range []string{"server_connect_failed", "server_tls_failed_check_ca_sni", "ca_read_failed", "ca_invalid_pem", "sstp_auth_failed_requires_mschapv2", "sstp_handshake_timeout", "sstp_handshake_failed_check_tls_credentials_and_mschapv2", "target_dns_failed", "turn_allocate_failed_check_credentials_and_rfc6062", "turn_connect_failed_check_peer_policy", "turn_probe_failed_check_server", "sstp_target_connect_failed"} {
			if value == known {
				code = known
				break
			}
		}
		for _, phase := range []string{"allocate", "create permission", "connect", "connection bind"} {
			if strings.HasPrefix(value, phase+":") {
				code = "turn_target_connect_failed"
				break
			}
		}
	}
	fmt.Fprintln(os.Stdout, "DS2API_PROXY_ERROR "+code)
}
