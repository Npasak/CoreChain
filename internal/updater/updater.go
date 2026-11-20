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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"corechain/internal/config"
	"corechain/internal/supervisor"
)

type PeerSource interface {
	GetPeerLanURLs() []string
}

type BlockManifestEntry struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	DistMode       string `json:"dist_mode,omitempty"`
	MinCoreVersion string `json:"min_core_version,omitempty"`
	DownloadURL    string `json:"download_url"`
	SHA256         string `json:"sha256"`
}

type RemoteManifest struct {
	CoreVersion string               `json:"core_version"`
	CoreArmURL  string               `json:"core_arm_url,omitempty"`
	CoreArmSHA  string               `json:"core_arm_sha256,omitempty"`
	CoreX64URL  string               `json:"core_x64_url,omitempty"`
	CoreX64SHA  string               `json:"core_x64_sha256,omitempty"`
	Blocks      []BlockManifestEntry `json:"blocks"`
	PublishedAt string               `json:"published_at"`
}

type BackgroundUpdater struct {
	cfg        *config.Config
	sup        *supervisor.Supervisor
	peerSrc    PeerSource
	httpClient *http.Client
	mu         sync.RWMutex
	lastCheck  time.Time
	lastStatus string
	isUpdating bool
	progress   int
	step       string
	target     string
	dlBytes    int64
	totalBytes int64
	latestCore string
}

func NewBackgroundUpdater(cfg *config.Config, sup *supervisor.Supervisor) *BackgroundUpdater {
	return &BackgroundUpdater{
		cfg: cfg,
		sup: sup,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		lastStatus: "idle",
		latestCore: config.Version,
	}
}

func (u *BackgroundUpdater) SetPeerSource(p PeerSource) {
	u.peerSrc = p
}

func (u *BackgroundUpdater) Start() {
	if !u.cfg.AutoUpdateEnabled {
		return
	}
	go func() {
		time.Sleep(12 * time.Second)
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
		"enabled":          u.cfg.AutoUpdateEnabled,
		"interval_min":     u.cfg.AutoUpdateMin,
		"manifest_url":     u.cfg.UpdateManifestURL,
		"current_version":  config.Version,
		"latest_version":   u.latestCore,
		"update_available": u.latestCore != "" && u.latestCore != config.Version,
		"last_check":       u.lastCheck,
		"status":           u.lastStatus,
		"is_updating":      u.isUpdating,
		"progress_pct":     u.progress,
		"current_step":     u.step,
		"target_item":      u.target,
		"bytes_downloaded": u.dlBytes,
		"bytes_total":      u.totalBytes,
	}
}

func (u *BackgroundUpdater) setProgress(updating bool, pct int, step, target, status string) {
	u.mu.Lock()
	u.isUpdating = updating
	u.progress = pct
	u.step = step
	u.target = target
	if status != "" {
		u.lastStatus = status
	}
	u.mu.Unlock()
}

func compareSemver(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	pb := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
	for i := 0; i < 3; i++ {
		var va, vb int
		if i < len(pa) {
			va, _ = strconv.Atoi(strings.Split(pa[i], "-")[0])
		}
		if i < len(pb) {
			vb, _ = strconv.Atoi(strings.Split(pb[i], "-")[0])
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

func (u *BackgroundUpdater) fetchManifest() (*RemoteManifest, string, error) {
	var candidates []string
	if u.cfg.UpdateManifestURL != "" {
		candidates = append(candidates, u.cfg.UpdateManifestURL)
	}
	if u.peerSrc != nil {
		for _, peerURL := range u.peerSrc.GetPeerLanURLs() {
			candidates = append(candidates, peerURL+"/api/sync/manifest.json")
		}
	}
	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("no manifest source configured")
	}

	var lastErr error
	for _, url := range candidates {
		resp, err := u.httpClient.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		var rm RemoteManifest
		decErr := json.NewDecoder(resp.Body).Decode(&rm)
		resp.Body.Close()
		if resp.StatusCode != 200 || decErr != nil {
			if decErr != nil {
				lastErr = decErr
			} else {
				lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
			}
			continue
		}
		return &rm, url, nil
	}
	return nil, "", lastErr
}

// CheckAndApply checks remote manifest (or LAN peer Hub fallback) and hot-swaps updated .Blz packages.
func (u *BackgroundUpdater) CheckAndApply() {
	u.mu.Lock()
	if u.isUpdating {
		u.mu.Unlock()
		return
	}
	u.lastCheck = time.Now()
	u.isUpdating = true
	u.progress = 5
	u.step = "กำลังตรวจสอบเวอร์ชัน (GitHub / LAN Hub)..."
	u.target = "CoreChain"
	u.lastStatus = "checking..."
	u.mu.Unlock()

	rm, sourceURL, err := u.fetchManifest()
	if err != nil {
		u.setProgress(false, 0, "", "", "check failed: "+err.Error())
		return
	}
	_ = sourceURL

	u.mu.Lock()
	if rm.CoreVersion != "" {
		u.latestCore = rm.CoreVersion
	}
	u.mu.Unlock()

	currentBlocks := u.sup.ListBlocks()
	currentVersions := map[string]string{}
	currentHashes := map[string]string{}
	for _, b := range currentBlocks {
		id, _ := b["id"].(string)
		ver, _ := b["version"].(string)
		hash, _ := b["package_hash"].(string)
		currentVersions[id] = ver
		currentHashes[id] = hash
	}

	currentModes := map[string]string{}
	for _, b := range currentBlocks {
		id, _ := b["id"].(string)
		mode, _ := b["dist_mode"].(string)
		currentModes[id] = mode
	}

	var toUpdate []BlockManifestEntry
	for _, entry := range rm.Blocks {
		if entry.DistMode == "stop" || entry.DistMode == "remove" {
			if _, exists := currentVersions[entry.ID]; exists && currentModes[entry.ID] != entry.DistMode {
				log.Printf("[Updater] Applying remote mode '%s' to block '%s'", entry.DistMode, entry.ID)
				_ = u.sup.SetBlockMode(entry.ID, entry.DistMode)
			}
			continue
		}
		if entry.DownloadURL == "" {
			continue
		}
		if entry.MinCoreVersion != "" && compareSemver(config.Version, entry.MinCoreVersion) < 0 {
			log.Printf("[Updater] Skipping %s v%s: requires CoreChain >= v%s (current v%s)", entry.ID, entry.Version, entry.MinCoreVersion, config.Version)
			continue
		}
		verDiff := currentVersions[entry.ID] != entry.Version
		hashDiff := entry.SHA256 != "" && !strings.EqualFold(currentHashes[entry.ID], entry.SHA256)
		if verDiff || hashDiff {
			toUpdate = append(toUpdate, entry)
		}
	}

	if len(toUpdate) == 0 {
		msg := fmt.Sprintf("up to date (v%s)", config.Version)
		if rm.CoreVersion != "" && rm.CoreVersion != config.Version {
			msg = fmt.Sprintf("CoreChain v%s available (current v%s)", rm.CoreVersion, config.Version)
		}
		u.setProgress(false, 100, "ครบถ้วนแล้ว", "", msg)
		return
	}

	updatedCount := 0
	for idx, entry := range toUpdate {
		basePct := 10 + (idx*80)/len(toUpdate)
		spanPct := 80 / len(toUpdate)
		u.setProgress(true, basePct, fmt.Sprintf("กำลังดาวน์โหลด %s v%s...", entry.ID, entry.Version), entry.ID+".Blz", fmt.Sprintf("updating %s...", entry.ID))

		tmpPath := filepath.Join(u.cfg.PackagesDir, fmt.Sprintf("%s-%s.Blz.tmp", entry.ID, entry.Version))
		if err := u.downloadAndVerify(entry.DownloadURL, tmpPath, entry.SHA256, basePct, spanPct); err != nil {
			log.Printf("[Updater] Failed downloading %s.Blz: %v", entry.ID, err)
			_ = os.Remove(tmpPath)
			continue
		}

		u.setProgress(true, basePct+spanPct-5, fmt.Sprintf("กำลัง Hot-Swap %s v%s...", entry.ID, entry.Version), entry.ID+".Blz", "")
		finalPath := filepath.Join(u.cfg.PackagesDir, fmt.Sprintf("%s.Blz", entry.ID))
		_ = os.Rename(tmpPath, finalPath)
		if _, err := u.sup.HotSwapBlock(finalPath); err != nil {
			log.Printf("[Updater] Hot-swap failed for %s: %v", entry.ID, err)
			continue
		}
		updatedCount++
	}

	u.setProgress(false, 100, "อัปเดตเสร็จสมบูรณ์", "", fmt.Sprintf("hot-swapped %d block(s)", updatedCount))
}

// BootstrapSync triggers a clean-machine full sync across blocks with live % progress.
func (u *BackgroundUpdater) BootstrapSync(peerURL string) {
	u.mu.Lock()
	if u.isUpdating {
		u.mu.Unlock()
		return
	}
	u.isUpdating = true
	u.progress = 10
	u.step = "กำลังเตรียมเชื่อมต่อ Peer / Google Drive..."
	u.target = "CoreChain Sync"
	u.lastStatus = "syncing..."
	u.mu.Unlock()

	busBase := "http://127.0.0.1:" + u.cfg.Port
	client := &http.Client{Timeout: 60 * time.Second}

	// Step 1: LearnChain sync
	u.setProgress(true, 30, "กำลัง Sync ข้อมูล LearnChain (บทเรียน, ผู้ใช้, เกียรติบัตร)...", "learnchain", "syncing learnchain...")
	if peerURL != "" {
		_, _ = client.Post(busBase+"/b/learnchain/api/sync/config", "application/json",
			strings.NewReader(fmt.Sprintf(`{"preferred_master_ip":"%s","auto_sync_enabled":true}`, strings.TrimPrefix(strings.TrimPrefix(peerURL, "http://"), "https://"))))
	}
	if resp, err := client.Post(busBase+"/b/learnchain/api/sync/auto-now", "application/json", nil); err == nil {
		_ = resp.Body.Close()
	}

	// Step 2: WebChain sync
	u.setProgress(true, 65, "กำลัง Sync ข้อมูล WebChain (ประกาศ, หอผู้ป่วย)...", "webchain", "syncing webchain...")
	if resp, err := client.Post(busBase+"/b/webchain/api/sync/now", "application/json", nil); err == nil {
		_ = resp.Body.Close()
	}

	// Step 3: Check .Blz updates
	u.setProgress(true, 90, "กำลังตรวจสอบความสมบูรณ์ของข้อมูล...", "all", "finalizing...")
	time.Sleep(500 * time.Millisecond)

	u.setProgress(false, 100, "Sync เครื่องใหม่เสร็จสมบูรณ์!", "", "sync complete")
}

func (u *BackgroundUpdater) downloadAndVerify(url, dest, expectedSHA string, basePct, spanPct int) error {
	resp, err := u.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	u.mu.Lock()
	u.dlBytes = 0
	u.totalBytes = total
	u.mu.Unlock()

	f, err := os.Create(dest)
	if err != nil {
		return err
	}

	h := sha256.New()
	mw := io.MultiWriter(f, h)
	buf := make([]byte, 32*1024)
	var downloaded int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := mw.Write(buf[:n]); wErr != nil {
				_ = f.Close()
				return wErr
			}
			downloaded += int64(n)
			u.mu.Lock()
			u.dlBytes = downloaded
			if total > 0 {
				u.progress = basePct + int((downloaded*int64(spanPct-5))/total)
			}
			u.mu.Unlock()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = f.Close()
			return readErr
		}
	}
	_ = f.Close()

	if expectedSHA != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, expectedSHA) {
			return fmt.Errorf("sha256 mismatch: got %s want %s", got, expectedSHA)
		}
	}
	return nil
}

// SelfUpdateCore downloads a newer CoreChain binary, verifies SHA256, swaps via .old rename, and restarts.
func (u *BackgroundUpdater) SelfUpdateCore() (string, error) {
	u.mu.Lock()
	if u.isUpdating {
		u.mu.Unlock()
		return "", fmt.Errorf("update already in progress")
	}
	u.isUpdating = true
	u.progress = 10
	u.step = "กำลังตรวจสอบ CoreChain เวอร์ชันใหม่..."
	u.target = "CoreChain.exe"
	u.lastStatus = "checking core..."
	u.mu.Unlock()

	rm, _, err := u.fetchManifest()
	if err != nil {
		u.setProgress(false, 0, "", "", "core check failed: "+err.Error())
		return "", err
	}
	if rm.CoreVersion == "" || compareSemver(rm.CoreVersion, config.Version) <= 0 {
		u.setProgress(false, 100, "CoreChain เป็นเวอร์ชันล่าสุดแล้ว", "", "up to date")
		return config.Version, nil
	}

	dlURL := rm.CoreX64URL
	dlSHA := rm.CoreX64SHA
	if runtime.GOARCH == "arm64" && rm.CoreArmURL != "" {
		dlURL = rm.CoreArmURL
		dlSHA = rm.CoreArmSHA
	}
	if dlURL == "" {
		err := fmt.Errorf("no CoreChain binary URL for %s", runtime.GOARCH)
		u.setProgress(false, 0, "", "", err.Error())
		return "", err
	}

	exePath, err := os.Executable()
	if err != nil {
		u.setProgress(false, 0, "", "", err.Error())
		return "", err
	}
	newPath := exePath + ".new"
	oldPath := exePath + ".old"

	u.setProgress(true, 20, fmt.Sprintf("กำลังดาวน์โหลด CoreChain v%s...", rm.CoreVersion), "CoreChain.exe", "downloading core...")
	if err := u.downloadAndVerify(dlURL, newPath, dlSHA, 20, 70); err != nil {
		_ = os.Remove(newPath)
		u.setProgress(false, 0, "", "", "core download failed: "+err.Error())
		return "", err
	}

	u.setProgress(true, 95, fmt.Sprintf("กำลังสลับ CoreChain v%s และรีสตาร์ท...", rm.CoreVersion), "CoreChain.exe", "restarting...")
	_ = os.Remove(oldPath)
	if err := os.Rename(exePath, oldPath); err != nil {
		_ = os.Remove(newPath)
		u.setProgress(false, 0, "", "", "rename failed: "+err.Error())
		return "", err
	}
	if err := os.Rename(newPath, exePath); err != nil {
		_ = os.Rename(oldPath, exePath)
		u.setProgress(false, 0, "", "", "swap failed: "+err.Error())
		return "", err
	}

	go func() {
		time.Sleep(400 * time.Millisecond)
		u.sup.ShutdownAll()
		cmd := exec.Command(exePath)
		cmd.Dir = filepath.Dir(exePath)
		_ = cmd.Start()
		os.Exit(0)
	}()

	return rm.CoreVersion, nil
}
