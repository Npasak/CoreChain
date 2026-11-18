package sync

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"corechain/internal/config"
	"corechain/internal/models"
	"corechain/internal/supervisor"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type PeerInfo struct {
	NodeID      string   `json:"node_id"`
	Hostname    string   `json:"hostname"`
	LanURL      string   `json:"lan_url"`
	LastSeenMs  int64    `json:"last_seen_ms"`
	Blocks      []string `json:"blocks"`
	CoreVersion string   `json:"core_version"`
	IsSelf      bool     `json:"is_self,omitempty"`
}

type MeshService struct {
	cfg        *config.Config
	db         *gorm.DB
	sup        *supervisor.Supervisor
	mu         sync.RWMutex
	nodeID     string
	hostname   string
	peers      map[string]PeerInfo
	httpClient *http.Client
}

func NewMeshService(cfg *config.Config, db *gorm.DB, sup *supervisor.Supervisor) *MeshService {
	var setting models.SystemSetting
	nodeID := "cc-node"
	if err := db.Where("key = ?", "node_id").First(&setting).Error; err == nil {
		nodeID = setting.Value
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "CoreChain-Node"
	}
	return &MeshService{
		cfg:      cfg,
		db:       db,
		sup:      sup,
		nodeID:   nodeID,
		hostname: host,
		peers:    make(map[string]PeerInfo),
		httpClient: &http.Client{
			Timeout: 4 * time.Second,
		},
	}
}

func (m *MeshService) GetLanURLs() []string {
	var urls []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return urls
	}
	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				if m.cfg.Port == "80" {
					urls = append(urls, fmt.Sprintf("http://%s", ipnet.IP.String()))
				} else {
					urls = append(urls, fmt.Sprintf("http://%s:%s", ipnet.IP.String(), m.cfg.Port))
				}
			}
		}
	}
	return urls
}

func (m *MeshService) Status(c *fiber.Ctx) error {
	lanURLs := m.GetLanURLs()
	primaryURL := "http://localhost:" + m.cfg.Port
	if len(lanURLs) > 0 {
		primaryURL = lanURLs[0]
	}

	blocks := m.sup.ListBlocks()
	blockIDs := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if id, ok := b["id"].(string); ok {
			blockIDs = append(blockIDs, id)
		}
	}

	self := PeerInfo{
		NodeID:      m.nodeID,
		Hostname:    m.hostname,
		LanURL:      primaryURL,
		LastSeenMs:  time.Now().UnixMilli(),
		Blocks:      blockIDs,
		CoreVersion: config.Version,
		IsSelf:      true,
	}

	m.mu.RLock()
	peerList := make([]PeerInfo, 0, len(m.peers)+1)
	peerList = append(peerList, self)
	for _, p := range m.peers {
		peerList = append(peerList, p)
	}
	m.mu.RUnlock()

	return c.JSON(fiber.Map{
		"node_id":      m.nodeID,
		"hostname":     m.hostname,
		"core_version": config.Version,
		"primary_url":  primaryURL,
		"lan_urls":     lanURLs,
		"peers":        peerList,
	})
}

func (m *MeshService) AddPeer(c *fiber.Ctx) error {
	var body struct {
		LanURL string `json:"lan_url"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.LanURL) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "lan_url is required"})
	}
	target := strings.TrimRight(strings.TrimSpace(body.LanURL), "/")
	resp, err := m.httpClient.Get(target + "/api/sync/status")
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": fmt.Sprintf("Cannot reach peer %s: %v", target, err)})
	}
	defer resp.Body.Close()

	var peerStatus struct {
		NodeID      string   `json:"node_id"`
		Hostname    string   `json:"hostname"`
		CoreVersion string   `json:"core_version"`
		Peers       []PeerInfo `json:"peers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&peerStatus); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": "Invalid peer response"})
	}
	var blocks []string
	for _, p := range peerStatus.Peers {
		if p.IsSelf {
			blocks = p.Blocks
			break
		}
	}
	info := PeerInfo{
		NodeID:      peerStatus.NodeID,
		Hostname:    peerStatus.Hostname,
		LanURL:      target,
		LastSeenMs:  time.Now().UnixMilli(),
		Blocks:      blocks,
		CoreVersion: peerStatus.CoreVersion,
	}
	m.mu.Lock()
	m.peers[peerStatus.NodeID] = info
	m.mu.Unlock()

	return c.JSON(fiber.Map{"ok": true, "peer": info})
}

// ServeBlzPackage lets LAN peers download .Blz packages for P2P update relay.
func (m *MeshService) ServeBlzPackage(c *fiber.Ctx) error {
	filename := filepath.Base(c.Params("filename"))
	p := filepath.Join(m.cfg.PackagesDir, filename)
	if _, err := os.Stat(p); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Package not found"})
	}
	return c.SendFile(p)
}
