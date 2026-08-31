package uidaemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	// EventSettingsHidden is posted to the host window when settings UI hides.
	EventSettingsHidden = 1
	// EventConfigSaved is posted when settings were saved successfully.
	EventConfigSaved = 2
	// EventUIExited is posted when the UI process exits unexpectedly.
	EventUIExited = 3

	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

// HostCallback delivers UI lifecycle events back to the host message loop.
type HostCallback func(event int)

// Manager owns the single UI daemon process from the host side.
type Manager struct {
	baseDir   string
	resolveUI func() string
	onEvent   HostCallback

	mu          sync.Mutex
	cmd         *exec.Cmd
	client      *Client
	callback    *CallbackServer
	callbackURL string
	callbackTok string
	owned       bool
	waitCh      chan error
	starting    bool
}

func NewManager(baseDir string, resolveUI func() string, onEvent HostCallback) *Manager {
	return &Manager{
		baseDir:   baseDir,
		resolveUI: resolveUI,
		onEvent:   onEvent,
	}
}

func (m *Manager) EnsureReady(ctx context.Context) error {
	if err := m.ensureCallback(); err != nil {
		return err
	}

	m.mu.Lock()
	if m.client != nil {
		client := m.client
		m.mu.Unlock()
		if _, err := client.Health(ctx); err == nil {
			return nil
		}
		m.mu.Lock()
		m.resetClientLocked()
	}

	if m.starting {
		m.mu.Unlock()
		return m.waitUntilReady(ctx)
	}
	m.starting = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.starting = false
		m.mu.Unlock()
	}()

	// Always prefer an owned daemon so callback credentials stay in sync.
	m.killStaleDaemon()
	return m.startProcess(ctx)
}

func (m *Manager) ShowSettings(ctx context.Context) error {
	if err := m.EnsureReady(ctx); err != nil {
		return err
	}
	client, err := m.currentClient()
	if err != nil {
		return err
	}
	return client.ShowSettings(ctx)
}

func (m *Manager) ShowTranslate(ctx context.Context) error {
	if err := m.EnsureReady(ctx); err != nil {
		return err
	}
	client, err := m.currentClient()
	if err != nil {
		return err
	}
	return client.ShowTranslate(ctx)
}

func (m *Manager) ShowResult(ctx context.Context, payload ResultPayload) error {
	if err := m.EnsureReady(ctx); err != nil {
		return err
	}
	client, err := m.currentClient()
	if err != nil {
		return err
	}
	return client.ShowResult(ctx, payload)
}

func (m *Manager) Hide(ctx context.Context) error {
	client, err := m.currentClient()
	if err != nil {
		return nil
	}
	return client.Hide(ctx)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	client := m.client
	cmd := m.cmd
	owned := m.owned
	waitCh := m.waitCh
	callback := m.callback
	m.mu.Unlock()

	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = client.Shutdown(ctx)
		cancel()
	}

	if owned && cmd != nil && cmd.Process != nil {
		done := make(chan struct{})
		go func() {
			if waitCh != nil {
				<-waitCh
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			if waitCh != nil {
				select {
				case <-waitCh:
				case <-time.After(1 * time.Second):
				}
			}
		}
	}

	if callback != nil {
		callback.Stop()
	}

	m.mu.Lock()
	m.resetLocked()
	m.mu.Unlock()
	RemoveEndpoint(m.baseDir)
}

func (m *Manager) ensureCallback() error {
	m.mu.Lock()
	if m.callback != nil && m.callbackURL != "" && m.callbackTok != "" {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	callback := NewCallbackServer(m.onEvent)
	url, token, err := callback.Start()
	if err != nil {
		return err
	}

	m.mu.Lock()
	if m.callback != nil {
		existingURL := m.callbackURL
		existingTok := m.callbackTok
		m.mu.Unlock()
		callback.Stop()
		if existingURL != "" && existingTok != "" {
			return nil
		}
		return errors.New("ui callback server unavailable")
	}
	m.callback = callback
	m.callbackURL = url
	m.callbackTok = token
	m.mu.Unlock()
	return nil
}

func (m *Manager) currentClient() (*Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		return nil, errors.New("ui daemon client unavailable")
	}
	return m.client, nil
}

func (m *Manager) killStaleDaemon() {
	endpoint, err := ReadEndpoint(m.baseDir)
	if err != nil {
		return
	}
	if endpoint.PID > 0 && processExists(endpoint.PID) {
		log.Printf("ui daemon killing stale process pid=%d", endpoint.PID)
		if proc, findErr := os.FindProcess(endpoint.PID); findErr == nil {
			_ = proc.Kill()
			time.Sleep(150 * time.Millisecond)
		}
	}
	RemoveEndpoint(m.baseDir)
}

func (m *Manager) startProcess(ctx context.Context) error {
	exePath := m.resolveUI()
	if exePath == "" {
		return errors.New("translate-ui.exe not found")
	}

	m.mu.Lock()
	callbackURL := m.callbackURL
	callbackTok := m.callbackTok
	m.mu.Unlock()
	if callbackURL == "" || callbackTok == "" {
		return errors.New("ui callback server not ready")
	}

	RemoveEndpoint(m.baseDir)
	cmd := exec.Command(
		exePath,
		"--daemon",
		"--callback-url", callbackURL,
		"--callback-token", callbackTok,
	)
	cmd.Dir = filepath.Dir(exePath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start translate-ui daemon: %w", err)
	}

	waitCh := make(chan error, 1)
	m.mu.Lock()
	m.cmd = cmd
	m.owned = true
	m.waitCh = waitCh
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		waitCh <- err
		close(waitCh)
		m.mu.Lock()
		if m.cmd != nil && m.cmd.Process != nil && m.cmd.Process.Pid == cmd.Process.Pid {
			m.resetClientLocked()
			m.cmd = nil
			m.owned = false
			m.waitCh = nil
		}
		m.mu.Unlock()
		RemoveEndpoint(m.baseDir)
		log.Printf("ui daemon process exited: pid=%d err=%v", cmd.Process.Pid, err)
		if m.onEvent != nil {
			m.onEvent(EventUIExited)
		}
	}()

	log.Printf("ui daemon process started: pid=%d", cmd.Process.Pid)
	return m.waitUntilReady(ctx)
}

func (m *Manager) waitUntilReady(ctx context.Context) error {
	deadline := time.Now().Add(20 * time.Second)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("wait for ui daemon ready timed out")
		}

		m.mu.Lock()
		waitCh := m.waitCh
		m.mu.Unlock()
		if waitCh != nil {
			select {
			case err := <-waitCh:
				if err != nil {
					return fmt.Errorf("ui daemon exited before ready: %w", err)
				}
				return errors.New("ui daemon exited before ready")
			default:
			}
		}

		endpoint, err := ReadEndpoint(m.baseDir)
		if err == nil {
			client := NewClient(endpoint)
			if status, healthErr := client.Health(ctx); healthErr == nil && status.Ready {
				m.mu.Lock()
				m.client = client
				m.mu.Unlock()
				log.Printf("ui daemon ready: url=%s", endpoint.URL)
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *Manager) resetClientLocked() {
	m.client = nil
}

func (m *Manager) resetLocked() {
	m.cmd = nil
	m.client = nil
	m.callback = nil
	m.callbackURL = ""
	m.callbackTok = ""
	m.owned = false
	m.waitCh = nil
}

func processExists(pid int) bool {
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
