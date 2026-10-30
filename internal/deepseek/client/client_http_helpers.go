package client

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ds2api/internal/config"

	"github.com/andybalholm/brotli"
)

// These are metadata responses, never SSE streams or downloaded file bytes.
const maxMetadataResponseBytes = 16 << 20

var ErrInvalidUpstreamResponse = errors.New("invalid upstream metadata response")
var ErrUpstreamResponseTooLarge = errors.New("upstream metadata response exceeds size limit")

func readResponseBody(resp *http.Response) ([]byte, error) {
	encoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	var reader io.Reader = resp.Body
	switch encoding {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer func() {
			if err := gz.Close(); err != nil {
				config.Logger.Warn("[deepseek] close gzip reader", "error", err)
			}
		}()
		reader = gz
	case "br":
		reader = brotli.NewReader(resp.Body)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxMetadataResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxMetadataResponseBytes {
		return nil, ErrUpstreamResponseTooLarge
	}
	return data, nil
}

// Keep non-2xx status codes usable even when a gateway sends an HTML error.
// A malformed success must never masquerade as an empty successful result.
func decodeMetadataResponse(data []byte, status int) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil || out == nil {
		if status < 200 || status >= 300 {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("%w: expected a JSON object", ErrInvalidUpstreamResponse)
	}
	return out, nil
}

func (c *Client) jsonHeaders(headers map[string]string) map[string]string {
	out := cloneStringMap(headers)
	out["Content-Type"] = "application/json"
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
