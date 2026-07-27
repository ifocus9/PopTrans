package uidaemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// CallbackServer receives UI-originated events on the host process.
type CallbackServer struct {
	token   string
	onEvent HostCallback

	mu       sync.Mutex
	listener net.Listener
	server   *http.Server
	url      string
}

func NewCallbackServer(onEvent HostCallback) *CallbackServer {
	return &CallbackServer{onEvent: onEvent}
}

func (s *CallbackServer) Start() (url, token string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return s.url, s.token, nil
	}

	token, err = NewToken()
	if err != nil {
		return "", "", err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", err
	}

	mux := http.NewServeMux()
	mux.HandleFunc(EndpointNotifyHidden, s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.onEvent != nil {
			s.onEvent(EventSettingsHidden)
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc(EndpointNotifyConfig, s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.onEvent != nil {
			s.onEvent(EventConfigSaved)
		}
		writeJSON(w, map[string]any{"ok": true})
	}))

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	s.token = token
	s.listener = listener
	s.server = server
	s.url = "http://" + listener.Addr().String()

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("ui callback server stopped: %v", serveErr)
		}
	}()
	log.Printf("ui callback server listening on %s", s.url)
	return s.url, s.token, nil
}

func (s *CallbackServer) Stop() {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.listener = nil
	s.mu.Unlock()
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func (s *CallbackServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(TokenHeader()) != s.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// NotifyClient posts events from UI process back to host callback server.
type NotifyClient struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

func NewNotifyClient(baseURL, token string) *NotifyClient {
	return &NotifyClient{
		httpClient: &http.Client{Timeout: 2 * time.Second},
		baseURL:    baseURL,
		token:      token,
	}
}

func (c *NotifyClient) Hidden(ctx context.Context) error {
	return c.post(ctx, EndpointNotifyHidden, map[string]any{"ok": true})
}

func (c *NotifyClient) ConfigSaved(ctx context.Context) error {
	return c.post(ctx, EndpointNotifyConfig, map[string]any{"ok": true})
}

func (c *NotifyClient) post(ctx context.Context, path string, body any) error {
	if c == nil || c.baseURL == "" || c.token == "" {
		return nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader(), c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(bytes.TrimSpace(respBody))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("notify %s: %s", path, msg)
	}
	return nil
}
