package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"webchain/config"
	"webchain/database"
	"webchain/handlers"
	"webchain/middleware"
	"webchain/version"

	"corechain/sdk/blocksdk"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

//go:embed dist/*
var embedDir embed.FS

func main() {
	fmt.Printf(`
======================================================
  WebChain: Internal Ward Communication Hub
  Special Nursing 2 (หอสงฆ์, สว11, สว12)
  Special Nursing 3 (8A, 8B, 9A, 9B, 9C)
  Version: %-12s Built: %s
======================================================
`, version.Version, version.BuildDate)

	env := blocksdk.LoadRuntimeEnv("webchain", "8081")
	cfg := config.LoadConfig()
	if pubPort := os.Getenv("CORECHAIN_PUBLIC_PORT"); pubPort != "" {
		cfg.Port = pubPort
	} else {
		cfg.Port = env.Port
	}
	cfg.DBPath = env.DBPath
	cfg.AutoOpenBrowser = false
	database.InitDB(cfg.DBPath)

	authHandler := handlers.NewAuthHandler()
	postHandler := handlers.NewPostHandler()
	linkHandler := handlers.NewLinkHandler()
	wardHandler := handlers.NewWardHandler()
	syncHandler := handlers.NewSyncHandler()
	godVaultHandler := handlers.NewGodVaultHandler()
	versionHandler := handlers.NewVersionHandler()

	app := fiber.New(fiber.Config{
		AppName:   "WebChain Intranet Hub",
		BodyLimit: 50 * 1024 * 1024,
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Auth-Token",
	}))

	setupCoreChainBridge(app, env)

	api := app.Group("/api")
	api.Use(middleware.OptionalUserMiddleware())

	// Version & Health
	api.Get("/version", versionHandler.GetVersion)

	// Auth & Users
	api.Post("/auth/login", authHandler.Login)
	api.Post("/auth/register", authHandler.Register)
	api.Get("/auth/me", authHandler.Me)
	api.Get("/users", middleware.RequireAdmin(), authHandler.ListUsers)
	api.Post("/users/:id/role", middleware.RequireAdmin(), authHandler.UpdateUserRole)

	// Posts & Stats
	api.Get("/posts", postHandler.ListPosts)
	api.Post("/posts", middleware.RequirePoster(), postHandler.CreatePost)
	api.Put("/posts/:id", middleware.RequirePoster(), postHandler.UpdatePost)
	api.Delete("/posts/:id", middleware.RequirePoster(), postHandler.DeletePost)
	api.Post("/posts/:id/applause", postHandler.ApplausePost)
	api.Get("/stats", postHandler.GetStats)

	// Ward Profiles & Monthly Birthdays
	api.Get("/wards", wardHandler.ListWards)
	api.Get("/wards/:code", wardHandler.GetWard)
	api.Put("/wards/:code", middleware.RequirePoster(), wardHandler.UpdateWard)
	api.Get("/birthdays", wardHandler.GetMonthlyBirthdays)
	api.Post("/birthdays/wish", wardHandler.SendBirthdayWish)

	// External Links (LearnChain & DocSearch)
	api.Get("/links", linkHandler.ListLinks)
	api.Post("/links", middleware.RequireAdmin(), linkHandler.UpsertLink)
	api.Get("/links/learnchain/ping", linkHandler.CheckLearnChainPing)

	// Master-Peer Sync & QR Code
	api.Get("/qr", syncHandler.GetQRCodeSVG)
	api.Get("/sync/status", syncHandler.GetStatus)
	api.Post("/sync/config", middleware.RequireAdmin(), syncHandler.UpdateConfig)
	api.Post("/sync/peers/add", middleware.RequireAdmin(), syncHandler.AddPeer)
	api.Get("/sync/export", syncHandler.ExportSnapshot)
	api.Post("/sync/import", syncHandler.ImportSnapshot)
	api.Post("/sync/now", syncHandler.SyncNow)

	// Hidden God Vault (therunrock only)
	god := api.Group("/god", middleware.RequireGodTherunrock())
	god.Get("/status", godVaultHandler.GetVaultStatus)
	god.Post("/backup", godVaultHandler.CreateBackup)
	god.Get("/backup/:filename", godVaultHandler.DownloadBackup)
	god.Post("/restore", godVaultHandler.RestoreBackup)
	god.Post("/reset", godVaultHandler.ResetSystem)

	// Embedded Static Frontend
	distFS, err := fs.Sub(embedDir, "dist")
	if err != nil {
		log.Fatalf("Failed to load embedded dist/: %v", err)
	}

	serveIndex := func(c *fiber.Ctx) error {
		data, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			return c.Status(404).SendString("index.html not found")
		}
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Send(data)
	}

	app.Get("/", serveIndex)
	app.Get("/special-2", serveIndex)
	app.Get("/special-3", serveIndex)

	app.Use("/", filesystem.New(filesystem.Config{
		Root:   http.FS(distFS),
		Browse: false,
	}))

	addr := "127.0.0.1:" + env.Port
	go func() {
		if err := app.Listen(addr); err != nil {
			log.Fatalf("Server stopped: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit
	_ = app.Shutdown()
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
