package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"webchain/config"
	"webchain/database"
	"webchain/models"

	"github.com/gofiber/fiber/v2"
)

type PeerNodeInfo struct {
	NodeID       string   `json:"node_id"`
	Hostname     string   `json:"hostname"`
	LanURL       string   `json:"lan_url"`
	LanURLs      []string `json:"lan_urls,omitempty"`
	LastSeenMs   int64    `json:"last_seen_ms"`
	LastSeenText string   `json:"last_seen_text"`
	PostsCount   int64    `json:"posts_count"`
	UsersCount   int64    `json:"users_count"`
	IsSelf       bool     `json:"is_self,omitempty"`
	IsMaster     bool     `json:"is_master"`
	ComputeScore float64  `json:"compute_score"`
	LatencyMs    int64    `json:"latency_ms"`
}

type SyncConfig struct {
	GasURL            string         `json:"gas_url"`
	AutoSyncEnabled   bool           `json:"auto_sync_enabled"`
	IntervalSec       int            `json:"interval_sec"`
	PreferredMasterIP string         `json:"preferred_master_ip,omitempty"`
	KnownPeers        []PeerNodeInfo `json:"known_peers,omitempty"`
}

type SyncSnapshot struct {
	App            string                 `json:"app"`
	NodeID         string                 `json:"node_id"`
	Hostname       string                 `json:"hostname,omitempty"`
	LanURL         string                 `json:"lan_url,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
	Users         []models.User          `json:"users"`
	Posts         []models.Post          `json:"posts"`
	ExternalLinks []models.ExternalLink  `json:"external_links"`
	Settings      []models.SystemSetting `json:"settings"`
	WardProfiles  []models.WardProfile   `json:"ward_profiles"`
	BirthdayWishes []models.BirthdayWish `json:"birthday_wishes"`
}

type SyncHandler struct {
	mu             sync.Mutex
	cfg            SyncConfig
	nodeID         string
	hostname       string
	lanURLs        []string
	activeNodes    map[string]PeerNodeInfo
	lastSyncAt     string
	lastSyncStatus string
	isSelfMaster   bool
	electedMaster  string
	computeScore   float64
	syncSuccess    int64
	syncFail       int64
}

var GlobalSyncHandler *SyncHandler

func NewSyncHandler() *SyncHandler {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "WebChain-Node"
	}
	port := "8889"
	gasURL := ""
	if config.AppConfig != nil {
		if config.AppConfig.Port != "" {
			port = config.AppConfig.Port
		}
		gasURL = config.AppConfig.GasSyncURL
	}
	lanURLs := discoverLocalURLs(port)
	sum := sha256.Sum256([]byte(hostname + "-" + strings.Join(lanURLs, ",")))
	nodeID := fmt.Sprintf("wc-%x", sum[:6])

	h := &SyncHandler{
		cfg: SyncConfig{
			GasURL:          gasURL,
			AutoSyncEnabled: true,
			IntervalSec:     30,
		},
		nodeID:         nodeID,
		hostname:       hostname,
		lanURLs:        lanURLs,
		activeNodes:    make(map[string]PeerNodeInfo),
		lastSyncStatus: "พร้อมซิงก์ (Master-Peer)",
		computeScore:   25.0,
		isSelfMaster:   true,
	}
	h.loadConfig()
	h.touchSelf()
	deduplicatePosts()
	GlobalSyncHandler = h
	go h.autoSyncLoop()
	return h
}

func (h *SyncHandler) TriggerImmediateSync() {
	go h.syncWithPeers()
}

func formatWebChainURL(host, port string) string {
	base := "http://" + host
	if port != "" && port != "80" {
		base = fmt.Sprintf("http://%s:%s", host, port)
	}
	if blockID := strings.TrimSpace(os.Getenv("CORECHAIN_BLOCK_ID")); blockID != "" {
		return base + "/b/" + blockID
	}
	return base
}

func discoverLocalURLs(port string) []string {
	var urls []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return []string{formatWebChainURL("localhost", port)}
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				urls = append(urls, formatWebChainURL(ipNet.IP.String(), port))
			}
		}
	}
	if len(urls) == 0 {
		urls = append(urls, formatWebChainURL("localhost", port))
	}
	return urls
}

func syncConfigPath() string {
	dbPath := "webchain.db"
	if config.AppConfig != nil && config.AppConfig.DBPath != "" {
		dbPath = config.AppConfig.DBPath
	}
	return dbPath + ".sync.json"
}

func (h *SyncHandler) loadConfig() {
	raw, err := os.ReadFile(syncConfigPath())
	if err != nil {
		return
	}
	var loaded SyncConfig
	if err := json.Unmarshal(raw, &loaded); err == nil {
		if loaded.IntervalSec < 10 {
			loaded.IntervalSec = 30
		}
		h.cfg = loaded
	}
}

func (h *SyncHandler) saveConfig() {
	raw, err := json.MarshalIndent(h.cfg, "", "  ")
	if err == nil {
		_ = os.WriteFile(syncConfigPath(), raw, 0644)
	}
}

func (h *SyncHandler) touchSelf() {
	var postsCount, usersCount int64
	database.DB.Model(&models.Post{}).Count(&postsCount)
	database.DB.Model(&models.User{}).Where("is_hidden = ?", false).Count(&usersCount)

	primaryURL := "http://localhost:8889"
	if len(h.lanURLs) > 0 {
		primaryURL = h.lanURLs[0]
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.activeNodes[h.nodeID] = PeerNodeInfo{
		NodeID:       h.nodeID,
		Hostname:     h.hostname,
		LanURL:       primaryURL,
		LanURLs:      h.lanURLs,
		LastSeenMs:   time.Now().UnixMilli(),
		LastSeenText: "ออนไลน์ (เครื่องนี้)",
		PostsCount:   postsCount,
		UsersCount:   usersCount,
		IsSelf:       true,
		IsMaster:     h.isSelfMaster,
		ComputeScore: h.computeScore,
	}
}

func (h *SyncHandler) autoSyncLoop() {
	for {
		h.mu.Lock()
		enabled := h.cfg.AutoSyncEnabled
		interval := h.cfg.IntervalSec
		h.mu.Unlock()

		if interval < 10 {
			interval = 30
		}
		time.Sleep(time.Duration(interval) * time.Second)
		if enabled {
			h.syncWithPeers()
		}
	}
}

func (h *SyncHandler) TriggerSync() {
	h.syncWithPeers()
}

func (h *SyncHandler) syncWithPeers() {
	h.touchSelf()
	h.pollGASHub()
	h.discoverPeersFromCoreChainAndLearnChain()

	h.mu.Lock()
	peers := append([]PeerNodeInfo{}, h.cfg.KnownPeers...)
	prefIP := strings.TrimSpace(h.cfg.PreferredMasterIP)
	h.mu.Unlock()

	if prefIP != "" {
		if strings.HasPrefix(prefIP, "http") {
			peers = append(peers, PeerNodeInfo{NodeID: "preferred", LanURL: prefIP})
		} else {
			peers = append(peers,
				PeerNodeInfo{NodeID: "preferred-cc", LanURL: "http://" + prefIP + "/b/webchain"},
				PeerNodeInfo{NodeID: "preferred-cc8080", LanURL: "http://" + prefIP + ":8080/b/webchain"},
				PeerNodeInfo{NodeID: "preferred-standalone", LanURL: "http://" + prefIP + ":8889"},
			)
		}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: nil, // Bypass organization HTTP_PROXY for direct hospital LAN sync
		},
	}
	anySuccess := false

	for _, peer := range peers {
		target := strings.TrimRight(peer.LanURL, "/")
		if target == "" || h.isOwnURL(target) {
			continue
		}
		start := time.Now()
		resp, err := client.Get(target + "/api/sync/export")
		if err != nil {
			h.mu.Lock()
			h.syncFail++
			h.mu.Unlock()
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		latency := time.Since(start).Milliseconds()

		var snap SyncSnapshot
		if err := json.Unmarshal(body, &snap); err != nil || snap.App != "WebChain" {
			continue
		}
		mergeSnapshot(snap)

		// Push our snapshot back so both Master & Peer stay identical
		localSnap := buildLocalSnapshot(h.nodeID)
		localSnap.Hostname = h.hostname
		if len(h.lanURLs) > 0 {
			localSnap.LanURL = h.lanURLs[0]
		}
		payload, _ := json.Marshal(localSnap)
		pushResp, err := client.Post(target+"/api/sync/import", "application/json", bytes.NewReader(payload))
		if err == nil && pushResp != nil {
			_ = pushResp.Body.Close()
		}

		activePosts, activeUsers := countActiveInSnapshot(snap)
		h.mu.Lock()
		h.activeNodes[snap.NodeID] = PeerNodeInfo{
			NodeID:       snap.NodeID,
			Hostname:     peer.Hostname,
			LanURL:       target,
			LastSeenMs:   time.Now().UnixMilli(),
			LastSeenText: "ออนไลน์",
			PostsCount:   activePosts,
			UsersCount:   activeUsers,
			IsMaster:     true,
			LatencyMs:    latency,
		}
		h.syncSuccess++
		anySuccess = true
		h.mu.Unlock()
	}

	h.mu.Lock()
	if anySuccess {
		h.lastSyncAt = time.Now().Format("15:04:05")
		h.lastSyncStatus = "ซิงก์สำเร็จ"
	} else if len(peers) == 0 {
		h.lastSyncStatus = "โหมดเดี่ยว (ยังไม่มี Peer)"
	}
	h.mu.Unlock()
}

// discoverPeersFromCoreChainAndLearnChain pulls peer IPs from CoreChain's UDP LAN Mesh
// and LearnChain's sync config so WebChain discovers peers automatically without manual setup.
func (h *SyncHandler) discoverPeersFromCoreChainAndLearnChain() {
	addCandidate := func(nodeID, hostname, rawURL string) {
		rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
		if rawURL == "" || h.isOwnURL(rawURL) {
			return
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, kp := range h.cfg.KnownPeers {
			if strings.EqualFold(strings.TrimRight(kp.LanURL, "/"), rawURL) {
				return
			}
		}
		h.cfg.KnownPeers = append(h.cfg.KnownPeers, PeerNodeInfo{
			NodeID:   nodeID,
			Hostname: hostname,
			LanURL:   rawURL,
		})
	}

	// 1. Ask CoreChain Mesh (/api/sync/status) & LearnChain live cross-subnet peers (/b/learnchain/api/sync/info)
	if busURL := strings.TrimSpace(os.Getenv("CORECHAIN_BUS_URL")); busURL != "" {
		busBase := strings.TrimRight(busURL, "/")
		c := &http.Client{Timeout: 3 * time.Second}
		if resp, err := c.Get(busBase + "/api/sync/status"); err == nil {
			var res struct {
				Peers []struct {
					NodeID   string `json:"node_id"`
					Hostname string `json:"hostname"`
					LanURL   string `json:"lan_url"`
					IsSelf   bool   `json:"is_self"`
				} `json:"peers"`
			}
			if json.NewDecoder(resp.Body).Decode(&res) == nil {
				for _, p := range res.Peers {
					if !p.IsSelf && p.LanURL != "" {
						addCandidate(p.NodeID, p.Hostname, strings.TrimRight(p.LanURL, "/")+"/b/webchain")
					}
				}
			}
			_ = resp.Body.Close()
		}

		// Live cross-subnet peers from LearnChain GAS signaling
		if resp, err := c.Get(busBase + "/b/learnchain/api/sync/info"); err == nil {
			var linfo struct {
				ActiveNodes []struct {
					NodeID   string   `json:"node_id"`
					Hostname string   `json:"hostname"`
					LanURL   string   `json:"lan_url"`
					LanURLs  []string `json:"lan_urls"`
					IsSelf   bool     `json:"is_self"`
				} `json:"active_nodes"`
				Config struct {
					KnownPeers []struct {
						NodeID   string `json:"node_id"`
						Hostname string `json:"hostname"`
						LanURL   string `json:"lan_url"`
					} `json:"known_peers"`
				} `json:"config"`
			}
			if json.NewDecoder(resp.Body).Decode(&linfo) == nil {
				addFromURL := func(hostname, rawURL string) {
					u, err := url.Parse(rawURL)
					if err != nil || u.Hostname() == "" {
						return
					}
					host := u.Hostname()
					addCandidate("wc-cc-"+host, hostname, "http://"+host+"/b/webchain")
					addCandidate("wc-cc8080-"+host, hostname, "http://"+host+":8080/b/webchain")
					addCandidate("wc-st-"+host, hostname, "http://"+host+":8889")
				}
				for _, an := range linfo.ActiveNodes {
					if an.IsSelf {
						continue
					}
					if an.LanURL != "" {
						addFromURL(an.Hostname, an.LanURL)
					}
					for _, u := range an.LanURLs {
						addFromURL(an.Hostname, u)
					}
				}
				for _, kp := range linfo.Config.KnownPeers {
					addFromURL(kp.Hostname, kp.LanURL)
				}
			}
			_ = resp.Body.Close()
		}
	}

	// 2. Check sibling data/learnchain.db.sync.json for known LAN peer IPs
	if config.AppConfig != nil && config.AppConfig.DBPath != "" {
		learnSyncPath := filepath.Join(filepath.Dir(config.AppConfig.DBPath), "learnchain.db.sync.json")
		if raw, err := os.ReadFile(learnSyncPath); err == nil {
			var lcfg struct {
				KnownPeers []struct {
					NodeID   string `json:"node_id"`
					Hostname string `json:"hostname"`
					LanURL   string `json:"lan_url"`
				} `json:"known_peers"`
			}
			if json.Unmarshal(raw, &lcfg) == nil {
				for _, kp := range lcfg.KnownPeers {
					u, err := url.Parse(kp.LanURL)
					if err != nil || u.Hostname() == "" {
						continue
					}
					host := u.Hostname()
					addCandidate("wc-cc-"+host, kp.Hostname, "http://"+host+"/b/webchain")
					addCandidate("wc-st-"+host, kp.Hostname, "http://"+host+":8889")
				}
			}
		}
	}
}

func (h *SyncHandler) pollGASHub() {
	h.mu.Lock()
	gasURL := strings.TrimSpace(h.cfg.GasURL)
	nodeID := h.nodeID
	hostname := h.hostname
	lanURLs := append([]string{}, h.lanURLs...)
	h.mu.Unlock()

	if gasURL == "" || !strings.Contains(gasURL, "script.google.com") {
		return
	}
	primaryURL := ""
	if len(lanURLs) > 0 {
		primaryURL = lanURLs[0]
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":   "heartbeat",
		"app":      "webchain",
		"node_id":  nodeID,
		"hostname": hostname,
		"lan_url":  primaryURL,
		"lan_urls": lanURLs,
	})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(gasURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var res struct {
		Nodes []PeerNodeInfo `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, n := range res.Nodes {
		if n.NodeID == "" || n.NodeID == h.nodeID || n.LanURL == "" {
			continue
		}
		found := false
		for i, kp := range h.cfg.KnownPeers {
			if kp.NodeID == n.NodeID || strings.EqualFold(kp.LanURL, n.LanURL) {
				h.cfg.KnownPeers[i].LanURL = n.LanURL
				h.cfg.KnownPeers[i].Hostname = n.Hostname
				found = true
				break
			}
		}
		if !found {
			h.cfg.KnownPeers = append(h.cfg.KnownPeers, n)
		}
	}
}

func (h *SyncHandler) isOwnURL(target string) bool {
	for _, u := range h.lanURLs {
		if strings.EqualFold(strings.TrimRight(u, "/"), target) {
			return true
		}
	}
	return strings.Contains(target, "localhost:") || strings.Contains(target, "127.0.0.1:")
}

func buildLocalSnapshot(nodeID string) SyncSnapshot {
	var users []models.User
	var posts []models.Post
	var links []models.ExternalLink
	var settings []models.SystemSetting
	var wards []models.WardProfile
	var wishes []models.BirthdayWish

	database.DB.Unscoped().Find(&users)
	database.DB.Unscoped().Find(&posts)
	database.DB.Find(&links)
	database.DB.Find(&settings)
	database.DB.Find(&wards)
	database.DB.Find(&wishes)

	return SyncSnapshot{
		App:            "WebChain",
		NodeID:         nodeID,
		Timestamp:      time.Now(),
		Users:          users,
		Posts:          posts,
		ExternalLinks:  links,
		Settings:       settings,
		WardProfiles:   wards,
		BirthdayWishes: wishes,
	}
}

func countActiveInSnapshot(snap SyncSnapshot) (int64, int64) {
	var posts, users int64
	for _, p := range snap.Posts {
		if !p.DeletedAt.Valid {
			posts++
		}
	}
	for _, u := range snap.Users {
		if !u.DeletedAt.Valid && !u.IsHidden {
			users++
		}
	}
	return posts, users
}

func deduplicatePosts() {
	var posts []models.Post
	database.DB.Order("created_at asc").Find(&posts)
	seen := make(map[string]bool)
	for _, p := range posts {
		key := strings.TrimSpace(p.Title) + "|" + p.Category + "|" + p.ScopeDept + "|" + p.AuthorName
		if seen[key] {
			database.DB.Unscoped().Delete(&p)
		} else {
			seen[key] = true
		}
	}
}

func mergeSnapshot(snap SyncSnapshot) int {
	merged := 0
	for _, u := range snap.Users {
		var existing models.User
		err := database.DB.Unscoped().Where("id = ?", u.ID).First(&existing).Error
		if err != nil {
			database.DB.Unscoped().Create(&u)
			merged++
		} else if u.UpdatedAt.After(existing.UpdatedAt) {
			database.DB.Unscoped().Save(&u)
			merged++
		}
	}
	for _, p := range snap.Posts {
		var existing models.Post
		err := database.DB.Unscoped().Where("id = ?", p.ID).First(&existing).Error
		if err != nil && p.Title != "" {
			// Deduplicate posts with identical content (e.g. initial seeded posts with different UUIDs)
			err = database.DB.Unscoped().Where("title = ? AND category = ? AND scope_dept = ? AND author_name = ?", p.Title, p.Category, p.ScopeDept, p.AuthorName).First(&existing).Error
		}
		if err != nil {
			database.DB.Unscoped().Create(&p)
			merged++
		} else {
			if p.ApplauseCount > existing.ApplauseCount {
				existing.ApplauseCount = p.ApplauseCount
			}
			if p.UpdatedAt.After(existing.UpdatedAt) {
				if existing.ApplauseCount > p.ApplauseCount {
					p.ApplauseCount = existing.ApplauseCount
				}
				p.ID = existing.ID
				database.DB.Unscoped().Save(&p)
				merged++
			} else if existing.ApplauseCount != p.ApplauseCount {
				database.DB.Unscoped().Save(&existing)
			}
		}
	}
	for _, l := range snap.ExternalLinks {
		var existing models.ExternalLink
		err := database.DB.Where("key = ?", l.Key).First(&existing).Error
		if err != nil || l.UpdatedAt.After(existing.UpdatedAt) {
			database.DB.Save(&l)
			merged++
		}
	}
	for _, w := range snap.WardProfiles {
		var existing models.WardProfile
		err := database.DB.Where("ward_code = ?", w.WardCode).First(&existing).Error
		if err != nil || w.UpdatedAt.After(existing.UpdatedAt) {
			database.DB.Save(&w)
			merged++
		}
	}
	for _, bw := range snap.BirthdayWishes {
		var existing models.BirthdayWish
		err := database.DB.Where("id = ?", bw.ID).First(&existing).Error
		if err != nil {
			database.DB.Create(&bw)
			merged++
		} else if bw.WishCount > existing.WishCount {
			existing.WishCount = bw.WishCount
			existing.UpdatedAt = bw.UpdatedAt
			database.DB.Save(&existing)
			merged++
		}
	}
	return merged
}

func (h *SyncHandler) GetStatus(c *fiber.Ctx) error {
	h.touchSelf()
	h.mu.Lock()
	defer h.mu.Unlock()

	nodes := make([]PeerNodeInfo, 0, len(h.activeNodes))
	for _, n := range h.activeNodes {
		nodes = append(nodes, n)
	}

	return c.JSON(fiber.Map{
		"success":          true,
		"node_id":          h.nodeID,
		"hostname":         h.hostname,
		"lan_urls":         h.lanURLs,
		"is_self_master":   h.isSelfMaster,
		"last_sync_at":     h.lastSyncAt,
		"last_sync_status": h.lastSyncStatus,
		"sync_success":     h.syncSuccess,
		"sync_fail":        h.syncFail,
		"config":           h.cfg,
		"nodes":            nodes,
	})
}

func (h *SyncHandler) UpdateConfig(c *fiber.Ctx) error {
	var body struct {
		GasURL            string `json:"gas_url"`
		AutoSyncEnabled   *bool  `json:"auto_sync_enabled"`
		IntervalSec       int    `json:"interval_sec"`
		PreferredMasterIP string `json:"preferred_master_ip"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูลไม่ถูกต้อง"})
	}

	h.mu.Lock()
	if body.GasURL != "" {
		h.cfg.GasURL = strings.TrimSpace(body.GasURL)
	}
	if body.AutoSyncEnabled != nil {
		h.cfg.AutoSyncEnabled = *body.AutoSyncEnabled
	}
	if body.IntervalSec >= 10 {
		h.cfg.IntervalSec = body.IntervalSec
	}
	h.cfg.PreferredMasterIP = strings.TrimSpace(body.PreferredMasterIP)
	h.saveConfig()
	h.mu.Unlock()

	return c.JSON(fiber.Map{"success": true, "config": h.cfg})
}

func (h *SyncHandler) AddPeer(c *fiber.Ctx) error {
	var body struct {
		Hostname string `json:"hostname"`
		LanURL   string `json:"lan_url"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.LanURL) == "" {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "กรุณาระบุ URL ของเครื่อง Peer เช่น http://192.168.1.50:8889"})
	}
	url := strings.TrimRight(strings.TrimSpace(body.LanURL), "/")
	if !strings.HasPrefix(url, "http") {
		url = "http://" + url
	}
	name := strings.TrimSpace(body.Hostname)
	if name == "" {
		name = url
	}

	h.mu.Lock()
	exists := false
	for _, p := range h.cfg.KnownPeers {
		if strings.EqualFold(p.LanURL, url) {
			exists = true
			break
		}
	}
	if !exists {
		h.cfg.KnownPeers = append(h.cfg.KnownPeers, PeerNodeInfo{
			NodeID:   url,
			Hostname: name,
			LanURL:   url,
		})
		h.saveConfig()
	}
	h.mu.Unlock()

	go h.syncWithPeers()
	return c.JSON(fiber.Map{"success": true, "known_peers": h.cfg.KnownPeers})
}

func (h *SyncHandler) ExportSnapshot(c *fiber.Ctx) error {
	snap := buildLocalSnapshot(h.nodeID)
	snap.Hostname = h.hostname
	if len(h.lanURLs) > 0 {
		snap.LanURL = h.lanURLs[0]
	}
	return c.JSON(snap)
}

func (h *SyncHandler) ImportSnapshot(c *fiber.Ctx) error {
	var snap SyncSnapshot
	if err := c.BodyParser(&snap); err != nil {
		return c.Status(400).JSON(fiber.Map{"success": false, "message": "ข้อมูล Snapshot ไม่ถูกต้อง"})
	}
	merged := mergeSnapshot(snap)

	if snap.NodeID != "" && snap.NodeID != h.nodeID {
		peerURL := strings.TrimRight(strings.TrimSpace(snap.LanURL), "/")
		if peerURL == "" {
			if rip := strings.TrimSpace(c.Get("X-Real-IP")); rip != "" && rip != "127.0.0.1" && rip != "::1" {
				peerURL = "http://" + rip + "/b/webchain"
			}
		}
		peerHost := snap.Hostname
		if peerHost == "" {
			peerHost = snap.NodeID
		}
		activePosts, activeUsers := countActiveInSnapshot(snap)
		h.mu.Lock()
		h.activeNodes[snap.NodeID] = PeerNodeInfo{
			NodeID:       snap.NodeID,
			Hostname:     peerHost,
			LanURL:       peerURL,
			LastSeenMs:   time.Now().UnixMilli(),
			LastSeenText: "ออนไลน์",
			PostsCount:   activePosts,
			UsersCount:   activeUsers,
		}
		if peerURL != "" && !h.isOwnURL(peerURL) {
			found := false
			for _, kp := range h.cfg.KnownPeers {
				if kp.NodeID == snap.NodeID || strings.EqualFold(strings.TrimRight(kp.LanURL, "/"), peerURL) {
					found = true
					break
				}
			}
			if !found {
				h.cfg.KnownPeers = append(h.cfg.KnownPeers, PeerNodeInfo{
					NodeID:   snap.NodeID,
					Hostname: peerHost,
					LanURL:   peerURL,
				})
			}
		}
		h.lastSyncAt = time.Now().Format("15:04:05")
		h.lastSyncStatus = "ซิงก์สำเร็จ"
		h.syncSuccess++
		h.mu.Unlock()
	}

	return c.JSON(fiber.Map{"success": true, "merged": merged})
}

func (h *SyncHandler) SyncNow(c *fiber.Ctx) error {
	go h.syncWithPeers()
	return c.JSON(fiber.Map{"success": true, "message": "เริ่มซิงก์ข้อมูลกับ Peer แล้ว"})
}
