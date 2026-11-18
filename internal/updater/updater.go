package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"corechain/internal/config"
	"corechain/internal/supervisor"
)

type BlockManifestEntry struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
}

type RemoteManifest struct {
	CoreVersion string               `json:"core_version"`
	Blocks      []BlockManifestEntry `json:"blocks"`
	PublishedAt string               `json:"published_at"`
}

type BackgroundUpdater struct {
	cfg        *config.Config
	sup        *supervisor.Supervisor
	httpClient *http.Client
	mu         sync.RWMutex
	lastCheck  time.Time
	lastStatus string
}

func NewBackgroundUpdater(cfg *config.Config, sup *supervisor.Supervisor) *BackgroundUpdater {
	return &BackgroundUpdater{
		cfg: cfg,
		sup: sup,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		lastStatus: "idle",
	}
}

func (u *BackgroundUpdater) Start() {
	if !u.cfg.AutoUpdateEnabled {
		return
	}
	go func() {
		// Initial check shortly after boot, then periodic
		time.Sleep(10 * time.Second)
		u.CheckAndApply()
		ticker := time.NewTicker(time.Duration(u.cfg.AutoUpdateMin) * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			u.CheckAndApply()
		}
	}()
}

func (u *BackgroundUpdater) Status() map[string]any {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return map[string]any{
		"enabled":      u.cfg.AutoUpdateEnabled,
		"interval_min": u.cfg.AutoUpdateMin,
		"manifest_url": u.cfg.UpdateManifestURL,
		"last_check":   u.lastCheck,
		"status":       u.lastStatus,
	}
}

// CheckAndApply checks remote manifest (if configured) and hot-swaps updated .Blz packages in the background.
func (u *BackgroundUpdater) CheckAndApply() {
	u.mu.Lock()
	u.lastCheck = time.Now()
	if u.cfg.UpdateManifestURL == "" {
		u.lastStatus = "ready (local .Blz watch mode)"
		u.mu.Unlock()
		return
	}
	u.lastStatus = "checking..."
	u.mu.Unlock()

	resp, err := u.httpClient.Get(u.cfg.UpdateManifestURL)
	if err != nil {
		u.setStatus("check failed: " + err.Error())
		return
	}
	defer resp.Body.Close()

	var rm RemoteManifest
	if err := json.NewDecoder(resp.Body).Decode(&rm); err != nil {
		u.setStatus("invalid manifest: " + err.Error())
		return
	}

	currentBlocks := u.sup.ListBlocks()
	currentVersions := map[string]string{}
	for _, b := range currentBlocks {
		id, _ := b["id"].(string)
		ver, _ := b["version"].(string)
		currentVersions[id] = ver
	}

	updatedCount := 0
	for _, entry := range rm.Blocks {
		if currentVersions[entry.ID] == entry.Version {
			continue
		}
		if entry.DownloadURL == "" {
			continue
		}
		tmpPath := filepath.Join(u.cfg.PackagesDir, fmt.Sprintf("%s-%s.Blz.tmp", entry.ID, entry.Version))
		if err := u.downloadAndVerify(entry.DownloadURL, tmpPath, entry.SHA256); err != nil {
			log.Printf("[Updater] Failed downloading %s.Blz: %v", entry.ID, err)
			_ = os.Remove(tmpPath)
			continue
		}
		finalPath := filepath.Join(u.cfg.PackagesDir, fmt.Sprintf("%s.Blz", entry.ID))
		_ = os.Rename(tmpPath, finalPath)
		if _, err := u.sup.HotSwapBlock(finalPath); err != nil {
			log.Printf("[Updater] Hot-swap failed for %s: %v", entry.ID, err)
			continue
		}
		updatedCount++
	}

	if updatedCount > 0 {
		u.setStatus(fmt.Sprintf("hot-swapped %d block(s) in background", updatedCount))
	} else {
		u.setStatus("up to date")
	}
}

func (u *BackgroundUpdater) setStatus(s string) {
	u.mu.Lock()
	u.lastStatus = s
	u.mu.Unlock()
}

func (u *BackgroundUpdater) downloadAndVerify(url, dest, expectedSHA string) error {
	resp, err := u.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	_ = f.Close()
	if err != nil {
		return err
	}
	if expectedSHA != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if got != expectedSHA {
			return fmt.Errorf("sha256 mismatch: got %s want %s", got, expectedSHA)
		}
	}
	return nil
}
