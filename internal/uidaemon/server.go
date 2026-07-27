package uidaemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// Server is the loopback control plane exposed by the single UI process.
type Server struct {
	baseDir string
	token   string
	handler Handler

	mu       sync.Mutex
	listener net.Listener
	server   *http.Server
	endpoint Endpoint
}

// Handler implements UI actions requested by the host.
type Handler interface {
	Status() Status
	ShowSettings() error
	ShowResult(payload ResultPayload) error
	Hide() error
	Quit() error
	OnHidden()
	OnConfigSaved()
}

func NewServer(baseDir string, handler Handler) *Server {
	return &Server{
		baseDir: baseDir,
		handler: handler,
	}
}

func (s *Server) Start() (Endpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return s.endpoint, nil
	}

	token, err := NewToken()
	if err != nil {
		return Endpoint{}, err
	}
	s.token = token

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Endpoint{}, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc(EndpointHealth, s.withAuth(s.handleHealth))
	mux.HandleFunc(EndpointShowSettings, s.withAuth(s.handleShowSettings))
	mux.HandleFunc(EndpointShowResult, s.withAuth(s.handleShowResult))
	mux.HandleFunc(EndpointHide, s.withAuth(s.handleHide))
	mux.HandleFunc(EndpointShutdown, s.withAuth(s.handleShutdown))
	mux.HandleFunc(EndpointNotifyHidden, s.withAuth(s.handleNotifyHidden))
	mux.HandleFunc(EndpointNotifyConfig, s.withAuth(s.handleNotifyConfig))

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	s.listener = listener
	s.server = server
	s.endpoint = Endpoint{
		URL:       "http://" + listener.Addr().String(),
		Token:     token,
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
	}

	if err := WriteEndpoint(s.baseDir, s.endpoint); err != nil {
		_ = listener.Close()
		s.listener = nil
		s.server = nil
		return Endpoint{}, err
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("ui daemon server stopped: %v", err)
		}
	}()

	log.Printf("ui daemon listening on %s", s.endpoint.URL)
	return s.endpoint, nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.listener = nil
	s.mu.Unlock()

	RemoveEndpoint(s.baseDir)
	if server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func (s *Server) Endpoint() Endpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.endpoint
}

func (s *Server) withAuth(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(TokenHeader()) != s.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.handler.Status())
}

func (s *Server) handleShowSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.handler.ShowSettings(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleShowResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload ResultPayload
	if err := jsonDecode(r, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.handler.ShowResult(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleHide(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.handler.Hide(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = s.handler.Quit()
	}()
}

func (s *Server) handleNotifyHidden(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handler.OnHidden()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleNotifyConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handler.OnConfigSaved()
	writeJSON(w, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func jsonDecode(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	return nil
}
