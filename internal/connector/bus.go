package connector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"corechain/internal/config"
	"corechain/internal/models"
	"corechain/internal/supervisor"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Bus provides Block-to-Block RPC and Pub/Sub Event routing.
type Bus struct {
	cfg        *config.Config
	db         *gorm.DB
	sup        *supervisor.Supervisor
	httpClient *http.Client
}

func NewBus(cfg *config.Config, db *gorm.DB, sup *supervisor.Supervisor) *Bus {
	return &Bus{
		cfg: cfg,
		db:  db,
		sup: sup,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

func (b *Bus) verifySecret(c *fiber.Ctx) bool {
	return c.Get("X-Core-Bus-Secret") == b.cfg.AuthSecret
}

// CallBlock routes an RPC call from one block (or the Launcher) to targetBlock's /_internal/rpc/:action.
func (b *Bus) CallBlock(c *fiber.Ctx) error {
	// Allow either internal bus secret or authenticated admin/user
	if !b.verifySecret(c) && c.Locals("unified_user") == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized connector call"})
	}
	target := c.Params("block")
	action := c.Params("action")

	port, ok := b.sup.GetWorkerPort(target)
	if !ok {
		return c.Status(503).JSON(fiber.Map{"error": fmt.Sprintf("Target block '%s' is offline", target)})
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/_internal/rpc/%s", port, action)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(c.Body()))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Core-Sender-Block", c.Get("X-Core-Sender-Block"))

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.Status(resp.StatusCode)
	c.Set("Content-Type", "application/json")
	return c.Send(body)
}

// PublishEvent receives an event from a block, logs it, and fans it out to subscribed blocks.
func (b *Bus) PublishEvent(c *fiber.Ctx) error {
	if !b.verifySecret(c) {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid bus secret"})
	}
	var body struct {
		Topic       string         `json:"topic"`
		SenderBlock string         `json:"sender_block"`
		Payload     map[string]any `json:"payload"`
	}
	if err := c.BodyParser(&body); err != nil || body.Topic == "" {
		return c.Status(400).JSON(fiber.Map{"error": "topic is required"})
	}

	payloadBytes, _ := json.Marshal(body.Payload)
	b.db.Create(&models.ConnectorEventLog{
		ID:          uuid.New().String(),
		Topic:       body.Topic,
		SenderBlock: body.SenderBlock,
		PayloadJSON: string(payloadBytes),
		CreatedAt:   time.Now(),
	})

	ports := b.sup.GetWorkersByTopic(body.Topic)
	rawEvent, _ := json.Marshal(body)
	for _, port := range ports {
		go func(p int) {
			url := fmt.Sprintf("http://127.0.0.1:%d/_internal/events", p)
			resp, err := b.httpClient.Post(url, "application/json", bytes.NewReader(rawEvent))
			if err == nil {
				_ = resp.Body.Close()
			}
		}(port)
	}

	return c.JSON(fiber.Map{
		"ok":          true,
		"subscribers": len(ports),
	})
}

// ListRecentEvents returns recent Connector Bus events for the Launcher UI.
func (b *Bus) ListRecentEvents(c *fiber.Ctx) error {
	var events []models.ConnectorEventLog
	b.db.Order("created_at desc").Limit(30).Find(&events)
	return c.JSON(fiber.Map{"events": events})
}
