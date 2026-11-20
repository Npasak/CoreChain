package main

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"corechain/internal/auth"
	"corechain/internal/config"
	"corechain/internal/connector"
	"corechain/internal/database"
	"corechain/internal/models"
	"corechain/internal/supervisor"
	mesh "corechain/internal/sync"
	"corechain/internal/tunnel"
	"corechain/internal/updater"
	"corechain/internal/vault"
	"corechain/sdk/blocksdk"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

//go:embed dist/*
var embedDist embed.FS

//go:embed corechain.ico
var embedFavicon []byte

func main() {
	fmt.Printf(`
======================================================
  CoreChain: Decentralized Modular Block Launcher
  Single Server • .Blz Packages • Isolated DBs
  Version: %-12s OS/Arch: %s/%s
======================================================
`, config.Version, runtime.GOOS, runtime.GOARCH)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	triggerQuit := func() {
		select {
		case quit <- os.Interrupt:
		default:
		}
	}

	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}

	cfg := config.Load()
	db := database.InitCoreDB(cfg)
	// Clear previous sessions on startup and whenever a fresh app window opens after being closed
	db.Where("1 = 1").Delete(&models.UserSession{})
	onFreshWindowOpen = func() {
		db.Where("1 = 1").Delete(&models.UserSession{})
	}

	authSvc := auth.NewService(db, cfg.AuthSecret)
	sup := supervisor.NewSupervisor(cfg, db, authSvc)
	bus := connector.NewBus(cfg, db, sup)
	meshSvc := mesh.NewMeshService(cfg, db, sup)
	tunnelSvc := tunnel.NewService(cfg)
	bgUpdater := updater.NewBackgroundUpdater(cfg, sup)
	bgUpdater.SetPeerSource(meshSvc)
	godVault := vault.NewGodVault(cfg, db)

	// Bind listener first (port 80 default, fallback to 8080/8888 if busy)
	var ln net.Listener
	var listenErr error
	for _, p := range []string{cfg.Port, "8080", "8888", "3000"} {
		if p == "" {
			continue
		}
		ln, listenErr = net.Listen("tcp", ":"+p)
		if listenErr == nil {
			if p != cfg.Port {
				log.Printf("[CoreChain] Port %s busy, switched to port %s", cfg.Port, p)
			}
			cfg.Port = p
			break
		}
	}
	if ln == nil {
		log.Fatalf("Failed to bind any port: %v", listenErr)
	}

	// Boot all installed .Blz blocks as isolated multi-core workers
	sup.BootAll()
	meshSvc.StartLANDiscovery()
	bgUpdater.Start()
	if cfg.AutoTunnel {
		tunnelSvc.Start()
	}

	app := fiber.New(fiber.Config{
		AppName:   "CoreChain Decentralized Launcher",
		BodyLimit: 100 * 1024 * 1024, // 100MB for .Blz packages & doc uploads
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Auth-Token",
	}))

	// Serve unified CoreChain icon & favicon across all blocks
	app.Get("/favicon.ico", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "image/x-icon")
		c.Set("Cache-Control", "public, max-age=86400")
		return c.Send(embedFavicon)
	})

	// Serve unified Pic/<block_id>/... directory directly
	app.Static("/Pic", cfg.PicDir)
	app.Static("/pic", cfg.PicDir)
	// Legacy /covers/:filename support -> Pic/learnchain/ first, then fallback to learnchain block (peers/GAS)
	app.Static("/covers", filepath.Join(cfg.PicDir, "learnchain"))

	proxyLearnCover := func(c *fiber.Ctx) error {
		fname := filepath.Base(c.Params("filename"))
		if fname == "" || fname == "." || fname == ".." {
			return c.SendStatus(404)
		}
		port, ok := sup.GetWorkerPort("learnchain")
		if !ok {
			return c.SendStatus(404)
		}
		target := fmt.Sprintf("http://127.0.0.1:%d/covers/%s", port, fname)
		client := &http.Client{Timeout: 25 * time.Second}
		resp, err := client.Get(target)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				_ = resp.Body.Close()
			}
			return c.SendStatus(404)
		}
		defer resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			c.Set("Content-Type", ct)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return c.SendStatus(502)
		}
		return c.Send(data)
	}
	app.Get("/covers/:filename", proxyLearnCover)
	app.Get("/Pic/learnchain/:filename", proxyLearnCover)
	app.Get("/pic/learnchain/:filename", proxyLearnCover)

	// CoreChain API
	api := app.Group("/api")
	api.Use(authSvc.OptionalUserMiddleware())

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"version": config.Version,
			"cpus":    runtime.NumCPU(),
		})
	})

	// Unified Auth
	api.Post("/auth/login", authSvc.Login)
	api.Post("/auth/register", authSvc.Register)
	api.Post("/auth/logout", authSvc.Logout)
	api.Get("/auth/me", authSvc.Me)
	api.Get("/auth/default-dest", func(c *fiber.Ctx) error {
		var s models.SystemSetting
		if err := db.Where("key = ?", "default_landing_dest").First(&s).Error; err == nil && s.Value != "" {
			return c.JSON(fiber.Map{"dest": s.Value})
		}
		return c.JSON(fiber.Map{"dest": "/"})
	})
	api.Post("/auth/default-dest", func(c *fiber.Ctx) error {
		if auth.GetUser(c) == nil {
			return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
		}
		var body struct {
			Dest string `json:"dest"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
		}
		dest := strings.TrimSpace(body.Dest)
		if dest != "/" && dest != "/b/webchain/special-2" && dest != "/b/webchain/special-3" {
			dest = "/"
		}
		db.Save(&models.SystemSetting{Key: "default_landing_dest", Value: dest})
		return c.JSON(fiber.Map{"ok": true, "dest": dest})
	})
	api.Get("/users", authSvc.RequireManager(), authSvc.ListUsers)
	api.Post("/users", authSvc.RequireManager(), authSvc.CreateUser)
	api.Patch("/users/:id", authSvc.RequireManager(), authSvc.UpdateUser)
	api.Delete("/users/:id", authSvc.RequireManager(), authSvc.DeleteUser)
	api.Post("/users/:id/block-role", authSvc.RequireManager(), authSvc.SetBlockRole)

	// Block Management & .Blz Hot-Swap
	api.Get("/blocks", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"blocks": sup.ListBlocks()})
	})
	api.Post("/blocks/upload", authSvc.RequireAdmin(), func(c *fiber.Ctx) error {
		file, err := c.FormFile("package")
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Missing .Blz package file field 'package'"})
		}
		dest := filepath.Join(cfg.PackagesDir, filepath.Base(file.Filename))
		if err := c.SaveFile(file, dest); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		manifest, err := sup.HotSwapBlock(dest)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"ok":       true,
			"message":  fmt.Sprintf("Hot-swapped %s v%s without restarting CoreChain", manifest.Name, manifest.Version),
			"manifest": manifest,
		})
	})
	api.Post("/blocks/:id/start", authSvc.RequireAdmin(), func(c *fiber.Ctx) error {
		id := c.Params("id")
		db.Model(&models.InstalledBlock{}).Where("id = ?", id).Update("enabled", true)
		if err := sup.StartBlock(id); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	api.Post("/blocks/:id/stop", authSvc.RequireAdmin(), func(c *fiber.Ctx) error {
		id := c.Params("id")
		db.Model(&models.InstalledBlock{}).Where("id = ?", id).Update("enabled", false)
		sup.StopBlock(id)
		return c.JSON(fiber.Map{"ok": true})
	})
	api.Post("/blocks/:id/restart", authSvc.RequireManager(), func(c *fiber.Ctx) error {
		id := c.Params("id")
		sup.StopBlock(id)
		if err := sup.StartBlock(id); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	api.Post("/blocks/:id/mode", authSvc.RequireManager(), func(c *fiber.Ctx) error {
		id := c.Params("id")
		var body struct {
			Mode string `json:"mode"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
		}
		mode := strings.ToLower(strings.TrimSpace(body.Mode))
		if mode == "remove" {
			u := auth.GetUser(c)
			if u == nil || (u.Username != "therunrock" && u.GlobalRole != "god") {
				return c.Status(403).JSON(fiber.Map{"error": "คำสั่ง Remove ใช้ได้เฉพาะ therunrock เท่านั้น"})
			}
		}
		if err := sup.SetBlockMode(id, mode); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true, "mode": mode})
	})
	api.Post("/blocks/:id/cpu", authSvc.RequireAdmin(), func(c *fiber.Ctx) error {
		id := c.Params("id")
		var body struct {
			CPUThreads int `json:"cpu_threads"`
		}
		if err := c.BodyParser(&body); err != nil || body.CPUThreads < 0 {
			return c.Status(400).JSON(fiber.Map{"error": "invalid cpu_threads"})
		}
		db.Model(&models.InstalledBlock{}).Where("id = ?", id).Update("cpu_threads", body.CPUThreads)
		sup.StopBlock(id)
		_ = sup.StartBlock(id)
		return c.JSON(fiber.Map{"ok": true, "cpu_threads": body.CPUThreads})
	})
	api.Post("/blocks/:id/local-only", authSvc.RequireAdmin(), func(c *fiber.Ctx) error {
		id := c.Params("id")
		var body struct {
			LocalOnly bool `json:"local_only"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
		}
		if err := sup.SetLocalOnly(id, body.LocalOnly); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true, "local_only": body.LocalOnly})
	})

	// Connector Bus (Block-to-Block RPC + Event Pub/Sub)
	api.Post("/connector/call/:block/:action", bus.CallBlock)
	api.Post("/connector/events", bus.PublishEvent)
	api.Get("/connector/events", bus.ListRecentEvents)

	// Decentralized LAN Mesh & .Blz Relay
	api.Get("/sync/status", meshSvc.Status)
	api.Get("/sync/manifest.json", meshSvc.ServeManifest)
	api.Post("/sync/peers/add", authSvc.RequireAdmin(), meshSvc.AddPeer)
	api.Get("/sync/packages/:filename", meshSvc.ServeBlzPackage)

	// Cloudflare 5G Public Tunnel
	api.Get("/tunnel/status", func(c *fiber.Ctx) error {
		return c.JSON(tunnelSvc.Status())
	})
	api.Post("/tunnel/start", func(c *fiber.Ctx) error {
		ip := c.IP()
		if auth.GetUser(c) == nil && ip != "127.0.0.1" && ip != "::1" {
			return c.Status(403).JSON(fiber.Map{"error": "Admin privileges required"})
		}
		tunnelSvc.Start()
		return c.JSON(fiber.Map{"ok": true, "tunnel": tunnelSvc.Status()})
	})
	api.Post("/tunnel/stop", authSvc.RequireAdmin(), func(c *fiber.Ctx) error {
		tunnelSvc.Stop()
		return c.JSON(fiber.Map{"ok": true, "tunnel": tunnelSvc.Status()})
	})

	// Background Auto-Updater
	api.Get("/updater/status", func(c *fiber.Ctx) error {
		return c.JSON(bgUpdater.Status())
	})
	api.Post("/updater/check", authSvc.RequireAdmin(), func(c *fiber.Ctx) error {
		go bgUpdater.CheckAndApply()
		return c.JSON(fiber.Map{"ok": true, "message": "Background update check started"})
	})
	api.Post("/updater/self-update", authSvc.RequireAdmin(), func(c *fiber.Ctx) error {
		ver, err := bgUpdater.SelfUpdateCore()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true, "version": ver, "message": "CoreChain self-update initiated"})
	})

	// Unified God Vault & Clean-Install Sync
	api.Post("/god/bootstrap-sync", authSvc.RequireAdmin(), func(c *fiber.Ctx) error {
		var body struct {
			PeerURL string `json:"peer_url"`
		}
		_ = c.BodyParser(&body)
		go bgUpdater.BootstrapSync(body.PeerURL)
		return c.JSON(fiber.Map{"ok": true, "message": "เริ่ม Sync เครื่องใหม่แล้ว"})
	})
	god := api.Group("/god", authSvc.RequireGod())
	god.Get("/status", godVault.Status)
	god.Post("/backup", godVault.CreateBackup)
	god.Get("/backup/:filename", godVault.DownloadBackup)

	// System-wide Login Gate: every block & CoreChain HUB requires CoreChain session
	requireCoreLoginGate := func(c *fiber.Ctx) error {
		if auth.GetUser(c) != nil {
			return c.Next()
		}
		p := c.Path()
		accept := c.Get("Accept")
		if strings.Contains(p, "/api/") || (strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html")) {
			return c.Status(401).JSON(fiber.Map{"error": "Authentication required", "login_url": "/login"})
		}
		return c.Redirect("/login?next=" + url.QueryEscape(c.OriginalURL()))
	}

	// Reverse Proxy to Isolated Block Workers (/b/:block/*)
	app.All("/b/:block", authSvc.OptionalUserMiddleware(), requireCoreLoginGate, sup.ProxyToBlock)
	app.All("/b/:block/*", authSvc.OptionalUserMiddleware(), requireCoreLoginGate, sup.ProxyToBlock)

	// Embedded Launcher Frontend
	distFS, err := fs.Sub(embedDist, "dist")
	if err != nil {
		log.Fatalf("Failed to load embedded dist/: %v", err)
	}
	app.Get("/login", func(c *fiber.Ctx) error {
		data, err := fs.ReadFile(distFS, "login.html")
		if err != nil {
			return c.Status(404).SendString("login.html not found")
		}
		c.Set("Content-Type", "text/html; charset=utf-8")
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		return c.Send(data)
	})
	app.Get("/", authSvc.OptionalUserMiddleware(), requireCoreLoginGate, func(c *fiber.Ctx) error {
		data, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			return c.Status(404).SendString("index.html not found")
		}
		c.Set("Content-Type", "text/html; charset=utf-8")
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		return c.Send(data)
	})
	app.Use("/", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		return c.Next()
	}, filesystem.New(filesystem.Config{
		Root:   http.FS(distFS),
		Browse: false,
	}))

	go func() {
		if err := app.Listener(ln); err != nil {
			log.Fatalf("CoreChain server stopped: %v", err)
		}
	}()

	localURL := "http://localhost"
	if cfg.Port != "80" {
		localURL += ":" + cfg.Port
	}
	log.Printf("CoreChain Launcher ready at %s (Package format: %s)", localURL, blocksdk.PackageExtension)

	if cfg.AutoOpenBrowser {
		go func() {
			time.Sleep(700 * time.Millisecond)
			openAppWindow(localURL)
		}()
	}

	// Start System Tray (runs on background thread on Windows)
	triggerRestart := func() {
		log.Println("Restarting CoreChain and all block workers...")
		closeAppWindows()
		tunnelSvc.Stop()
		sup.ShutdownAll()
		_ = app.Shutdown()
		if exePath, err := os.Executable(); err == nil {
			cmd := exec.Command(exePath)
			cmd.Dir = filepath.Dir(exePath)
			_ = cmd.Start()
		}
		os.Exit(0)
	}

	go runSystemTray(localURL, func() {
		go func() {
			bgUpdater.CheckAndApply()
			_, _ = bgUpdater.SelfUpdateCore()
		}()
	}, triggerRestart, triggerQuit)

	<-quit
	log.Println("Shutting down CoreChain and all block workers...")
	closeAppWindows()
	tunnelSvc.Stop()
	sup.ShutdownAll()
	_ = app.Shutdown()
}
