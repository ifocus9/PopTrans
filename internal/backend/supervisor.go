package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"translate-plugin/internal/config"
)

// createNoWindow 对应 Windows 的 CREATE_NO_WINDOW 进程创建标志。
// 让后端拥有独立控制台、不继承前端控制台，从而不与前端处于同一控制台进程组。
const createNoWindow = 0x08000000
const serverPortEnv = "TRANSLATE_SERVER_PORT"

type Supervisor struct {
	baseDir string
	client  *Client
	cmd     *exec.Cmd
	owned   bool
	waitCh  chan error
}

func NewSupervisor(baseDir string, client *Client) *Supervisor {
	return &Supervisor{
		baseDir: baseDir,
		client:  client,
	}
}

func (s *Supervisor) EnsureRunning(ctx context.Context, onStatus func(string)) error {
	log.Printf("supervisor ensure running: base_dir=%s", s.baseDir)

	health, err := s.client.Health(ctx)
	shouldStart := true
	if err == nil && health.Status == "ok" {
		log.Printf(
			"supervisor found existing backend: translator_ready=%t translator_status=%q ocr_loaded=%t",
			health.TranslatorReady,
			health.TranslatorStatus,
			health.OCRLoaded,
		)
		if health.TranslatorStatus != "" {
			onStatus(health.TranslatorStatus)
		}
		if initErr := translatorInitializationError(health); initErr != nil {
			return initErr
		}
		if health.TranslatorReady {
			return nil
		}
		shouldStart = false
		log.Printf("supervisor existing backend reachable but translator is still loading")
	} else {
		log.Printf("supervisor initial health unavailable: %v", err)
	}

	if shouldStart {
		onStatus("starting local api service...")

		cmd, backendName, err := s.backendCommand()
		if err != nil {
			return err
		}
		cmd.Dir = s.baseDir
		// 捕获选中文本时前端会模拟发送 Ctrl+C。若后端与前端共享控制台进程组，
		// 该 Ctrl+C 会被控制台解释为 CTRL_C_EVENT 广播给整个进程组，
		// 使后端被终止（退出码 0xC000013A / STATUS_CONTROL_C_EXIT）。
		// 用 CREATE_NO_WINDOW 让后端脱离前端控制台，彻底隔离该信号。
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: createNoWindow,
		}

		var backendLogFile *os.File
		cfg, cfgErr := config.Load(s.baseDir)
		loggingEnabled := cfgErr == nil && cfg.LoggingEnabled
		serverPort := config.DefaultServerPort
		if cfgErr == nil {
			serverPort = cfg.ServerPort
		}
		cmd.Env = environmentWith(os.Environ(), serverPortEnv, strconv.Itoa(serverPort))
		if loggingEnabled {
			backendLogPath := filepath.Join(s.baseDir, "translate.log")
			backendLogFile, err = os.OpenFile(backendLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				log.Printf("supervisor open backend log failed: %v", err)
				backendLogFile = nil
				cmd.Stdout = io.Discard
				cmd.Stderr = io.Discard
			} else {
				cmd.Stdout = backendLogFile
				cmd.Stderr = backendLogFile
				log.Printf("supervisor redirecting backend stdout/stderr to %s", backendLogPath)
			}
		} else {
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
		}

		if err := cmd.Start(); err != nil {
			if backendLogFile != nil {
				_ = backendLogFile.Close()
			}
			log.Printf("supervisor failed to start %s: %v", backendName, err)
			return fmt.Errorf("start %s: %w", backendName, err)
		}

		s.cmd = cmd
		s.owned = true
		s.waitCh = make(chan error, 1)
		log.Printf("supervisor started backend process: pid=%d", cmd.Process.Pid)

		go func() {
			waitErr := cmd.Wait()
			if backendLogFile != nil {
				_ = backendLogFile.Close()
			}
			if waitErr != nil {
				log.Printf("supervisor backend process exited with error: pid=%d err=%v", cmd.Process.Pid, waitErr)
			} else {
				log.Printf("supervisor backend process exited cleanly: pid=%d", cmd.Process.Pid)
			}
			s.waitCh <- waitErr
			close(s.waitCh)
		}()
	}

	deadline := time.Now().Add(2 * time.Minute)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++

		if s.waitCh != nil {
			select {
			case waitErr, ok := <-s.waitCh:
				s.waitCh = nil
				s.cmd = nil
				s.owned = false
				if ok && waitErr != nil {
					log.Printf("supervisor detected backend exit before ready on attempt %d: %v", attempt, waitErr)
					return fmt.Errorf("api server exited before ready: %w", waitErr)
				}
				log.Printf("supervisor detected backend exit before ready on attempt %d", attempt)
				return fmt.Errorf("api server exited before ready")
			default:
			}
		}

		health, err = s.client.Health(ctx)
		if err == nil && health.Status == "ok" {
			log.Printf(
				"supervisor health check success on attempt %d: translator_ready=%t translator_status=%q ocr_loaded=%t",
				attempt,
				health.TranslatorReady,
				health.TranslatorStatus,
				health.OCRLoaded,
			)
			if health.TranslatorStatus != "" {
				onStatus(health.TranslatorStatus)
			}
			if initErr := translatorInitializationError(health); initErr != nil {
				if s.owned {
					s.Stop()
				}
				return initErr
			}
			if health.TranslatorReady {
				log.Printf("supervisor backend ready on attempt %d", attempt)
				return nil
			}
			log.Printf("supervisor backend reachable but translator not ready on attempt %d", attempt)
		} else {
			log.Printf("supervisor health check attempt %d failed: %v", attempt, err)
		}

		select {
		case <-ctx.Done():
			log.Printf("supervisor context done while waiting for backend: %v", ctx.Err())
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	log.Printf("supervisor backend start timed out after %d attempts", attempt)
	return fmt.Errorf("wait for ai engine ready timed out")
}

func (s *Supervisor) backendCommand() (*exec.Cmd, string, error) {
	enginePath := filepath.Join(s.baseDir, "ai_engine.exe")
	if _, err := os.Stat(enginePath); err == nil {
		return exec.Command(enginePath), "ai_engine.exe", nil
	}

	scriptPath := filepath.Join(s.baseDir, "backend", "api_server.py")
	if _, err := os.Stat(scriptPath); err == nil {
		return exec.Command("python", "-m", "backend.api_server"), "python -m backend.api_server", nil
	}

	return nil, "", fmt.Errorf("ai backend not found: expected %s", enginePath)
}

func environmentWith(environ []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if strings.HasPrefix(strings.ToUpper(entry), strings.ToUpper(prefix)) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, prefix+value)
}

func translatorInitializationError(health Health) error {
	if strings.HasPrefix(health.TranslatorStatus, "翻译引擎初始化失败") {
		return errors.New(health.TranslatorStatus)
	}
	return nil
}

func (s *Supervisor) Stop() {
	if s.cmd == nil || !s.owned || s.cmd.Process == nil {
		log.Printf("supervisor stop skipped: no owned backend process")
		return
	}

	log.Printf("supervisor stopping backend process: pid=%d", s.cmd.Process.Pid)
	if err := terminateProcessTree(s.cmd.Process); err != nil {
		log.Printf("supervisor process tree termination failed, using direct kill: pid=%d err=%v", s.cmd.Process.Pid, err)
		_ = s.cmd.Process.Kill()
	}

	if s.waitCh != nil {
		select {
		case waitErr := <-s.waitCh:
			log.Printf("supervisor backend process wait finished during stop: err=%v", waitErr)
		case <-time.After(5 * time.Second):
			log.Printf("supervisor timed out waiting for backend process to exit during stop")
		}
	}

	s.cmd = nil
	s.owned = false
	s.waitCh = nil
	log.Printf("supervisor backend process stopped")
}

func (s *Supervisor) Restart(ctx context.Context, onStatus func(string)) error {
	s.Stop()
	return s.EnsureRunning(ctx, onStatus)
}

func terminateProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}

	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(process.Pid), "/T", "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("taskkill: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
