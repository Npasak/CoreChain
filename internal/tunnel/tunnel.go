package tunnel

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"corechain/internal/config"
)

var tryCloudflareRe = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)

type Service struct {
	cfg       *config.Config
	mu        sync.RWMutex
	status    string // stopped, downloading, starting, running, error
	publicURL string
	lastError string
	startedAt time.Time
	cmd       *exec.Cmd
}

func NewService(cfg *config.Config) *Service {
	return &Service{
		cfg:    cfg,
		status: "stopped",
	}
}

func (s *Service) Status() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"status":     s.status,
		"public_url": s.publicURL,
		"error":      s.lastError,
		"started_at": s.startedAt,
	}
}

func (s *Service) Start() {
	s.mu.Lock()
	if s.status == "running" || s.status == "starting" || s.status == "downloading" {
		s.mu.Unlock()
		return
	}
	s.status = "starting"
	s.publicURL = ""
	s.lastError = ""
	s.mu.Unlock()

	go s.run()
}

func (s *Service) Stop() {
	s.mu.Lock()
	cmd := s.cmd
	s.cmd = nil
	s.status = "stopped"
	s.publicURL = ""
	s.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (s *Service) run() {
	binPath, err := s.ensureBinary()
	if err != nil {
		s.setError("ไม่พบและดาวน์โหลด cloudflared ไม่สำเร็จ: " + err.Error())
		return
	}

	target := fmt.Sprintf("http://127.0.0.1:%s", s.cfg.Port)
	cmd := exec.Command(binPath, "tunnel", "--no-autoupdate", "--protocol", "http2", "--url", target)
	configureHiddenProcess(cmd)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.setError(err.Error())
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.setError(err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		s.setError("เปิด cloudflared ไม่สำเร็จ: " + err.Error())
		return
	}

	s.mu.Lock()
	s.cmd = cmd
	s.startedAt = time.Now()
	s.mu.Unlock()

	scanPipe := func(r io.Reader) {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			line := sc.Text()
			if m := tryCloudflareRe.FindString(line); m != "" {
				s.mu.Lock()
				s.publicURL = m
				s.status = "running"
				s.lastError = ""
				s.mu.Unlock()
				log.Printf("[Tunnel] Cloudflare 5G Public URL ready: %s", m)
			}
		}
	}
	go scanPipe(stderr)
	go scanPipe(stdout)

	err = cmd.Wait()
	s.mu.Lock()
	wasStopped := s.status == "stopped"
	s.cmd = nil
	s.publicURL = ""
	if !wasStopped {
		s.status = "error"
		if err != nil {
			s.lastError = "cloudflared หยุดทำงาน: " + err.Error()
		} else {
			s.lastError = "cloudflared หยุดทำงาน"
		}
	}
	s.mu.Unlock()
}

func (s *Service) setError(msg string) {
	s.mu.Lock()
	s.status = "error"
	s.lastError = msg
	s.publicURL = ""
	s.mu.Unlock()
}

func (s *Service) ensureBinary() (string, error) {
	// 1. Check PATH
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, nil
	}

	// 2. Check local data/cloudflared.exe
	exeName := "cloudflared"
	dlURL := "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64"
	if runtime.GOOS == "windows" {
		exeName = "cloudflared.exe"
		// cloudflared-windows-amd64.exe runs natively on x64 and via emulation on Windows 11 ARM64
		dlURL = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"
	}
	localPath := filepath.Join(s.cfg.DataDir, exeName)
	if info, err := os.Stat(localPath); err == nil && info.Size() > 1024*1024 {
		return localPath, nil
	}

	// 3. Auto-download cloudflared
	s.mu.Lock()
	s.status = "downloading"
	s.mu.Unlock()

	log.Printf("[Tunnel] Downloading cloudflared from %s ...", dlURL)
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(dlURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpPath := localPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	_ = f.Close()
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmpPath, 0755)
	}
	if err := os.Rename(tmpPath, localPath); err != nil {
		return "", err
	}
	s.mu.Lock()
	if strings.EqualFold(s.status, "downloading") {
		s.status = "starting"
	}
	s.mu.Unlock()
	return localPath, nil
}
