package uidaemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to the single UI daemon over loopback HTTP.
type Client struct {
	httpClient *http.Client
	endpoint   Endpoint
}

func NewClient(endpoint Endpoint) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 3 * time.Second},
		endpoint:   endpoint,
	}
}

func (c *Client) Endpoint() Endpoint {
	return c.endpoint
}

func (c *Client) Health(ctx context.Context) (Status, error) {
	var status Status
	if err := c.do(ctx, http.MethodGet, EndpointHealth, nil, &status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (c *Client) ShowSettings(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, EndpointShowSettings, map[string]any{}, nil)
}

func (c *Client) ShowTranslate(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, EndpointShowTranslate, map[string]any{}, nil)
}

func (c *Client) ShowResult(ctx context.Context, payload ResultPayload) error {
	return c.do(ctx, http.MethodPost, EndpointShowResult, payload, nil)
}

func (c *Client) Hide(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, EndpointHide, map[string]any{}, nil)
}

func (c *Client) Shutdown(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, EndpointShutdown, map[string]any{}, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint.URL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(TokenHeader(), c.endpoint.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(bytes.TrimSpace(respBody))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("ui daemon %s: %s", path, msg)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}
