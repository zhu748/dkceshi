package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	trans "ds2api/internal/deepseek/transport"
)

func (c *Client) postJSON(ctx context.Context, doer trans.Doer, fallback trans.Doer, url string, headers map[string]string, payload any) (map[string]any, error) {
	body, status, err := c.postJSONWithStatus(ctx, doer, fallback, url, headers, payload)
	if err != nil {
		return nil, err
	}
	if status == 0 {
		return nil, errors.New("request failed")
	}
	return body, nil
}

func (c *Client) postJSONWithStatus(ctx context.Context, doer trans.Doer, fallback trans.Doer, url string, headers map[string]string, payload any) (map[string]any, int, error) {
	// payload 为 nil 时发送空 body，对齐真实 Android App 行为（如 chat_session/create）
	var body []byte
	if payload != nil {
		var err error
		body, err = dsprotocol.MarshalAppJSON(url, payload)
		if err != nil {
			return nil, 0, err
		}
	}
	headers = c.jsonHeaders(headers)

	// 原有 utls + net/http2 路径
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := doer.Do(req)
	if err != nil {
		if trans.UseFramedH2() {
			return nil, 0, err
		}
		config.Logger.Warn("[deepseek] fingerprint request failed, fallback to std transport", "url", url, "error", err)
		req2, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if reqErr != nil {
			return nil, 0, reqErr
		}
		for k, v := range headers {
			req2.Header.Set(k, v)
		}
		resp, err = fallback.Do(req2)
		if err != nil {
			return nil, 0, err
		}
	}
	defer func() { _ = resp.Body.Close() }()
	payloadBytes, err := readResponseBody(resp)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	out := map[string]any{}
	if len(payloadBytes) > 0 {
		if err := json.Unmarshal(payloadBytes, &out); err != nil {
			config.Logger.Warn("[deepseek] json parse failed", "url", url, "status", resp.StatusCode, "content_encoding", resp.Header.Get("Content-Encoding"), "preview", preview(payloadBytes))
		}
	}
	return out, resp.StatusCode, nil
}

func (c *Client) getJSONWithStatus(ctx context.Context, doer trans.Doer, url string, headers map[string]string) (map[string]any, int, error) {
	headers = cloneStringMap(headers)
	for key := range headers {
		if strings.EqualFold(key, "Content-Type") || strings.EqualFold(key, "Content-Length") {
			delete(headers, key)
		}
	}
	clients := c.requestClientsFromContext(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := doer.Do(req)
	if err != nil {
		if trans.UseFramedH2() {
			return nil, 0, err
		}
		config.Logger.Warn("[deepseek] fingerprint GET request failed, fallback to std transport", "url", url, "error", err)
		req2, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if reqErr != nil {
			return nil, 0, reqErr
		}
		for k, v := range headers {
			req2.Header.Set(k, v)
		}
		resp, err = clients.fallback.Do(req2)
		if err != nil {
			return nil, 0, err
		}
	}
	defer func() { _ = resp.Body.Close() }()
	payloadBytes, err := readResponseBody(resp)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	out := map[string]any{}
	if len(payloadBytes) > 0 {
		if err := json.Unmarshal(payloadBytes, &out); err != nil {
			config.Logger.Warn("[deepseek] json parse failed", "url", url, "status", resp.StatusCode, "content_encoding", resp.Header.Get("Content-Encoding"), "preview", preview(payloadBytes))
		}
	}
	return out, resp.StatusCode, nil
}
