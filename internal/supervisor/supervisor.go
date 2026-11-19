package supervisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"corechain/internal/auth"
	"corechain/internal/config"
	"corechain/internal/models"
	"corechain/sdk/blocksdk"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// WorkerState represents a running or stopped block worker managed by CoreChain.
type WorkerState struct {
	Manifest    blocksdk.Manifest `json:"manifest"`
	Port        int               `json:"port"`
	PID         int               `json:"pid"`
	Status      string            `json:"status"` // running, starting, stopped, updating, error
	LastError   string            `json:"last_error,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	PackageHash string            `json:"package_hash"`
	CPUThreads  int               `json:"cpu_threads"`
	cmd         *exec.Cmd
	stopCh      chan struct{}
}

// Supervisor manages all .Blz block workers, multi-core allocation, health, and hot-swapping.
type Supervisor struct {
	cfg        *config.Config
	db         *gorm.DB
	authSvc    *auth.Service
	mu         sync.RWMutex
	workers    map[string]*WorkerState
	httpClient *http.Client
	shutting   bool
}

func NewSupervisor(cfg *config.Config, db *gorm.DB, authSvc *auth.Service) *Supervisor {
	return &Supervisor{
		cfg:     cfg,
		db:      db,
		authSvc: authSvc,
		workers: make(map[string]*WorkerState),
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

// BootAll scans packages/*.Blz and installed_blocks/*, installs/updates as needed, and starts all enabled blocks.
func (s *Supervisor) BootAll() {
	s.installPackagesFromDir()
	s.startInstalledBlocks()
	go s.healthLoop()
}

func (s *Supervisor) installPackagesFromDir() {
	entries, err := os.ReadDir(s.cfg.PackagesDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), blocksdk.PackageExtension) {
			continue
		}
		pkgPath := filepath.Join(s.cfg.PackagesDir, e.Name())
		hash, err := blocksdk.FileSHA256(pkgPath)
		if err != nil {
			continue
		}
		tmpDir := filepath.Join(s.cfg.BlocksDir, "_scan_tmp")
		_ = os.RemoveAll(tmpDir)
		manifest, err := blocksdk.UnpackBlz(pkgPath, tmpDir)
		if err != nil {
			log.Printf("[Supervisor] Failed to inspect %s: %v", e.Name(), err)
			_ = os.RemoveAll(tmpDir)
			continue
		}
		var rec models.InstalledBlock
		err = s.db.Where("id = ?", manifest.ID).First(&rec).Error
		targetDir := filepath.Join(s.cfg.BlocksDir, manifest.ID)
		if err != nil || rec.PackageHash != hash || !dirExists(targetDir) {
			_ = os.RemoveAll(targetDir)
			if err := os.Rename(tmpDir, targetDir); err != nil {
				log.Printf("[Supervisor] Failed to install %s: %v", manifest.ID, err)
				_ = os.RemoveAll(tmpDir)
				continue
			}
			s.upsertInstalledRecord(manifest, hash)
			log.Printf("[Supervisor] Installed .Blz package: %s v%s", manifest.Name, manifest.Version)
		} else {
			_ = os.RemoveAll(tmpDir)
		}
	}
}

func (s *Supervisor) upsertInstalledRecord(m *blocksdk.Manifest, hash string) {
	now := time.Now()
	var rec models.InstalledBlock
	err := s.db.Where("id = ?", m.ID).First(&rec).Error
	if err != nil {
		rec = models.InstalledBlock{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			Version:     m.Version,
			Icon:        m.Icon,
			Color:       m.Color,
			Enabled:     true,
			CPUThreads:  m.CPUThreads,
			SortOrder:   m.Order,
			PackageHash: hash,
			InstalledAt: now,
			UpdatedAt:   now,
		}
		s.db.Create(&rec)
	} else {
		s.db.Model(&rec).Updates(map[string]any{
			"name":         m.Name,
			"description":  m.Description,
			"version":      m.Version,
			"icon":         m.Icon,
			"color":        m.Color,
			"package_hash": hash,
			"updated_at":   now,
		})
	}
}

func (s *Supervisor) startInstalledBlocks() {
	entries, err := os.ReadDir(s.cfg.BlocksDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasSuffix(e.Name(), ".old") || strings.HasSuffix(e.Name(), ".staging") {
			continue
		}
		blockDir := filepath.Join(s.cfg.BlocksDir, e.Name())
		manifest, err := blocksdk.ReadManifest(blockDir)
		if err != nil {
			continue
		}
		var rec models.InstalledBlock
		if err := s.db.Where("id = ?", manifest.ID).First(&rec).Error; err == nil && !rec.Enabled {
			s.mu.Lock()
			s.workers[manifest.ID] = &WorkerState{
				Manifest:    *manifest,
				Status:      "stopped",
				PackageHash: rec.PackageHash,
				CPUThreads:  rec.CPUThreads,
			}
			s.mu.Unlock()
			continue
		}
		if err := s.StartBlock(manifest.ID); err != nil {
			log.Printf("[Supervisor] Error starting block %s: %v", manifest.ID, err)
		}
	}
}

// StartBlock launches a block worker on a free loopback port with dedicated CPU threads.
func (s *Supervisor) StartBlock(blockID string) error {
	blockDir := filepath.Join(s.cfg.BlocksDir, blockID)
	manifest, err := blocksdk.ReadManifest(blockDir)
	if err != nil {
		return err
	}

	var rec models.InstalledBlock
	_ = s.db.Where("id = ?", blockID).First(&rec).Error
	threads := rec.CPUThreads
	if threads <= 0 {
		threads = manifest.CPUThreads
	}
	if threads <= 0 {
		threads = max(1, runtime.NumCPU()/2)
	}

	port, err := findFreePort()
	if err != nil {
		return err
	}

	exeName := manifest.Entrypoint
	if runtime.GOOS != "windows" && strings.HasSuffix(exeName, ".exe") {
		exeName = strings.TrimSuffix(exeName, ".exe")
	}
	exePath := filepath.Join(blockDir, exeName)
	absExe, err := filepath.Abs(exePath)
	if err != nil {
		return err
	}
	absDB, _ := filepath.Abs(filepath.Join(s.cfg.DataDir, blockID+".db"))
	absPic, _ := filepath.Abs(filepath.Join(s.cfg.PicDir, blockID))
	absDataDir, _ := filepath.Abs(filepath.Join(s.cfg.DataDir, blockID+"_files"))
	_ = os.MkdirAll(absPic, 0755)
	_ = os.MkdirAll(absDataDir, 0755)

	cmd := exec.Command(absExe)
	cmd.Dir = blockDir
	configureHiddenProcess(cmd)
	cmd.Env = append(os.Environ(),
		"CORECHAIN_BLOCK_ID="+blockID,
		"CORECHAIN_PORT="+strconv.Itoa(port),
		"CORECHAIN_DB_PATH="+absDB,
		"CORECHAIN_PIC_DIR="+absPic,
		"CORECHAIN_DATA_DIR="+absDataDir,
		"CORECHAIN_BUS_URL=http://127.0.0.1:"+s.cfg.Port,
		"CORECHAIN_PUBLIC_PORT="+s.cfg.Port,
		"CORECHAIN_BUS_SECRET="+s.cfg.AuthSecret,
		"CORECHAIN_CPU_THREADS="+strconv.Itoa(threads),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", blockID, err)
	}

	state := &WorkerState{
		Manifest:    *manifest,
		Port:        port,
		PID:         cmd.Process.Pid,
		Status:      "starting",
		StartedAt:   time.Now(),
		PackageHash: rec.PackageHash,
		CPUThreads:  threads,
		cmd:         cmd,
		stopCh:      make(chan struct{}),
	}

	s.mu.Lock()
	s.workers[blockID] = state
	s.mu.Unlock()

	// Monitor exit
	go func(st *WorkerState) {
		err := st.cmd.Wait()
		s.mu.Lock()
		if st.Status != "stopped" && st.Status != "updating" && !s.shutting {
			st.Status = "error"
			if err != nil {
				st.LastError = err.Error()
			}
		}
		s.mu.Unlock()
	}(state)

	// Wait briefly for /_internal/health
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/_internal/health", port)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := s.httpClient.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				s.mu.Lock()
				state.Status = "running"
				s.mu.Unlock()
				log.Printf("[Supervisor] Block '%s' v%s running on 127.0.0.1:%d (PID %d, %d CPU threads)",
					blockID, manifest.Version, port, state.PID, threads)
				return nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}

	s.mu.Lock()
	state.Status = "running"
	s.mu.Unlock()
	return nil
}

// StopBlock gracefully stops a running block worker without touching CoreChain or other blocks.
func (s *Supervisor) StopBlock(blockID string) {
	s.mu.Lock()
	st, ok := s.workers[blockID]
	if !ok || st.cmd == nil || st.cmd.Process == nil {
		s.mu.Unlock()
		return
	}
	st.Status = "stopped"
	proc := st.cmd.Process
	s.mu.Unlock()

	_ = proc.Signal(os.Interrupt)
	time.Sleep(400 * time.Millisecond)
	_ = proc.Kill()
}

// HotSwapBlock installs or updates a .Blz package with zero downtime for CoreChain and other blocks!
func (s *Supervisor) HotSwapBlock(blzPath string) (*blocksdk.Manifest, error) {
	hash, err := blocksdk.FileSHA256(blzPath)
	if err != nil {
		return nil, err
	}

	stagingDir := filepath.Join(s.cfg.BlocksDir, fmt.Sprintf("_staging_%d", time.Now().UnixNano()))
	manifest, err := blocksdk.UnpackBlz(blzPath, stagingDir)
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, err
	}

	// Backup block DB before swap
	dbPath := filepath.Join(s.cfg.DataDir, manifest.ID+".db")
	if _, err := os.Stat(dbPath); err == nil {
		backupName := fmt.Sprintf("%s_preupdate_%s.db", manifest.ID, time.Now().Format("20060102_150405"))
		_ = copyFileSimple(dbPath, filepath.Join(s.cfg.BackupsDir, backupName))
	}

	s.mu.Lock()
	if st, ok := s.workers[manifest.ID]; ok {
		st.Status = "updating"
	}
	s.mu.Unlock()

	// Stop only this block
	s.StopBlock(manifest.ID)
	time.Sleep(300 * time.Millisecond)

	targetDir := filepath.Join(s.cfg.BlocksDir, manifest.ID)
	oldDir := targetDir + ".old"
	_ = os.RemoveAll(oldDir)
	if dirExists(targetDir) {
		_ = os.Rename(targetDir, oldDir)
	}
	if err := os.Rename(stagingDir, targetDir); err != nil {
		// Rollback directory
		if dirExists(oldDir) {
			_ = os.Rename(oldDir, targetDir)
		}
		return nil, err
	}
	_ = os.RemoveAll(oldDir)

	s.upsertInstalledRecord(manifest, hash)
	if err := s.StartBlock(manifest.ID); err != nil {
		return nil, err
	}
	log.Printf("[Supervisor] Hot-swapped block '%s' to v%s without restarting CoreChain!", manifest.ID, manifest.Version)
	return manifest, nil
}

// ShutdownAll cleanly stops all block workers when CoreChain exits.
func (s *Supervisor) ShutdownAll() {
	s.mu.Lock()
	s.shutting = true
	ids := make([]string, 0, len(s.workers))
	for id := range s.workers {
		ids = append(ids, id)
	}
	s.mu.Unlock()

	for _, id := range ids {
		s.StopBlock(id)
	}
}

func (s *Supervisor) healthLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.RLock()
		if s.shutting {
			s.mu.RUnlock()
			return
		}
		var restartIDs []string
		for id, st := range s.workers {
			if st.Status == "error" {
				restartIDs = append(restartIDs, id)
			}
		}
		s.mu.RUnlock()

		for _, id := range restartIDs {
			log.Printf("[Supervisor] Auto-restarting crashed block: %s", id)
			_ = s.StartBlock(id)
		}
	}
}

// ListBlocks returns all block states enriched with live /_internal/stats.
func (s *Supervisor) ListBlocks() []map[string]any {
	s.mu.RLock()
	list := make([]*WorkerState, 0, len(s.workers))
	for _, st := range s.workers {
		list = append(list, st)
	}
	s.mu.RUnlock()

	out := make([]map[string]any, 0, len(list))
	for _, st := range list {
		item := map[string]any{
			"id":           st.Manifest.ID,
			"name":         st.Manifest.Name,
			"description":  st.Manifest.Description,
			"version":      st.Manifest.Version,
			"icon":         st.Manifest.Icon,
			"color":        st.Manifest.Color,
			"order":        st.Manifest.Order,
			"status":       st.Status,
			"port":         st.Port,
			"pid":          st.PID,
			"cpu_threads":  st.CPUThreads,
			"package_hash": st.PackageHash,
			"url":          "/b/" + st.Manifest.ID + "/",
		}
		if st.Status == "running" && st.Port > 0 {
			if stats, err := s.fetchBlockStats(st.Port); err == nil {
				item["stats"] = stats
			}
		}
		out = append(out, item)
	}
	return out
}

func (s *Supervisor) fetchBlockStats(port int) (*blocksdk.BlockStats, error) {
	c := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/_internal/stats", port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var st blocksdk.BlockStats
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, err
	}
	return &st, nil
}

// GetWorkerPort returns the loopback port of a running block.
func (s *Supervisor) GetWorkerPort(blockID string) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.workers[blockID]
	if !ok || st.Status != "running" || st.Port == 0 {
		return 0, false
	}
	return st.Port, true
}

// GetWorkersByTopic returns all running block IDs subscribed to a topic.
func (s *Supervisor) GetWorkersByTopic(topic string) []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ports []int
	for _, st := range s.workers {
		if st.Status != "running" || st.Port == 0 {
			continue
		}
		for _, t := range st.Manifest.SubscribedTopics {
			if t == topic || t == "*" {
				ports = append(ports, st.Port)
				break
			}
		}
	}
	return ports
}

// ProxyToBlock forwards HTTP requests under /b/:block/* to the block's loopback worker with Unified Auth headers.
func (s *Supervisor) ProxyToBlock(c *fiber.Ctx) error {
	blockID := c.Params("block")
	port, ok := s.GetWorkerPort(blockID)
	if !ok {
		return c.Status(503).JSON(fiber.Map{
			"error": fmt.Sprintf("Block '%s' is not running", blockID),
		})
	}

	subPath := "/" + c.Params("*")
	if subPath == "/" && !strings.HasSuffix(c.Path(), "/") {
		return c.Redirect("/b/" + blockID + "/")
	}

	targetURL := fmt.Sprintf("http://127.0.0.1:%d%s", port, subPath)
	if q := string(c.Request().URI().QueryString()); q != "" {
		targetURL += "?" + q
	}

	req, err := http.NewRequest(c.Method(), targetURL, bytes.NewReader(c.Body()))
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}

	c.Request().Header.VisitAll(func(k, v []byte) {
		key := string(k)
		if strings.EqualFold(key, "Host") || strings.EqualFold(key, "Connection") {
			return
		}
		req.Header.Add(key, string(v))
	})
	req.Header.Set("X-Forwarded-Prefix", "/b/"+blockID)
	req.Header.Set("X-Real-IP", c.IP())
	req.Header.Set("X-Forwarded-For", c.IP())
	s.authSvc.InjectGatewayHeaders(c, blockID, req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": fmt.Sprintf("Block '%s' unreachable: %v", blockID, err)})
	}
	defer resp.Body.Close()

	for k, vals := range resp.Header {
		if strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
			continue
		}
		for _, v := range vals {
			c.Append(k, v)
		}
	}
	c.Status(resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return c.Send(body)
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func copyFileSimple(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_ = os.MkdirAll(filepath.Dir(dst), 0755)
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
