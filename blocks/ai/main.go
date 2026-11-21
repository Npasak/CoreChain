package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"corechain/sdk/blocksdk"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

//go:embed dist/* npu_server.py
var embedDist embed.FS

type AIConfig struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	EndpointURL string `json:"endpoint_url"`
	ModelName   string `json:"model_name"`
	SystemNote  string `json:"system_note"`
}

type ChatLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Mode      string    `json:"mode"`
	Prompt    string    `json:"prompt"`
	Response  string    `json:"response"`
	Engine    string    `json:"engine"`
	LatencyMs int64     `json:"latency_ms"`
	CreatedAt time.Time `json:"created_at"`
}

type NPUHardwareInfo struct {
	DeviceName      string `json:"device_name"`
	DeviceStatus    string `json:"device_status"`
	Arch            string `json:"arch"`
	DirectMLReady   bool   `json:"directml_ready"`
	QNNReady        bool   `json:"qnn_ready"`
	PythonPath      string `json:"python_path,omitempty"`
	LlamaExe        string `json:"llama_exe,omitempty"`
	LlamaRunning    bool   `json:"llama_running"`
	NPULLMRunning   bool   `json:"npu_llm_running"`
	NPULLMModel     string `json:"npu_llm_model,omitempty"`
	FoundryCLI      bool   `json:"foundry_cli"`
	ActiveEngine    string `json:"active_engine"`
	ActiveEndpoint  string `json:"active_endpoint"`
	ActiveModel     string `json:"active_model"`
	DetectedAtUnix  int64  `json:"detected_at_unix"`
}

type GGUFModelInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size string `json:"size"`
}

var (
	llamaMu  sync.Mutex
	llamaCmd *exec.Cmd
	npuMu    sync.Mutex
	npuCmd   *exec.Cmd
)

func findQNNModelDir() string {
	var roots []string
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots,
			filepath.Join(home, "OneDrive", "Documents", "LLModel"),
			filepath.Join(home, "Documents", "LLModel"),
		)
	}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(root, e.Name())
			if _, err := os.Stat(filepath.Join(dir, "genai_config.json")); err == nil {
				return dir
			}
		}
	}
	return ""
}

func startNPUServer(dataDir string) {
	py := findQNNPython()
	modelDir := findQNNModelDir()
	if py == "" || modelDir == "" {
		return
	}
	scriptBytes, err := embedDist.ReadFile("npu_server.py")
	if err != nil {
		return
	}
	_ = os.MkdirAll(dataDir, 0755)
	scriptPath := filepath.Join(dataDir, "npu_server.py")
	if err := os.WriteFile(scriptPath, scriptBytes, 0644); err != nil {
		return
	}

	npuMu.Lock()
	if npuCmd != nil && npuCmd.Process != nil {
		npuMu.Unlock()
		return
	}
	cmd := exec.Command(py, scriptPath, modelDir, "8098")
	hideCmdWindow(cmd)
	if err := cmd.Start(); err == nil {
		npuCmd = cmd
		log.Printf("[AI] Started Hexagon NPU LLM Server (%s)", filepath.Base(modelDir))
	}
	npuMu.Unlock()

	hc := &http.Client{Timeout: 800 * time.Millisecond}
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		if resp, err := hc.Get("http://127.0.0.1:8098/v1/models"); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				hwMu.Lock()
				lastHWScan = time.Time{}
				hwMu.Unlock()
				break
			}
		}
	}
}

func findLlamaServer() string {
	candidates := []string{
		`C:\llama\llama-server.exe`,
	}
	if home, err := os.UserHomeDir(); err == nil {
		matches, _ := filepath.Glob(filepath.Join(home, ".lmstudio", "extensions", "backends", "llama.cpp-win-arm64-*", "llama-server.exe"))
		candidates = append(candidates, matches...)
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("llama-server"); err == nil {
		return p
	}
	return ""
}

func scanGGUFModels() []GGUFModelInfo {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, "OneDrive", "Documents", "LLModel"),
			filepath.Join(home, "Documents", "LLModel"),
			filepath.Join(home, ".cache", "huggingface", "hub"),
		)
	}
	seen := map[string]bool{}
	var out []GGUFModelInfo
	for _, root := range dirs {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(d.Name()), ".gguf") {
				return nil
			}
			if strings.Contains(strings.ToLower(d.Name()), "mmproj") || strings.Contains(strings.ToLower(d.Name()), "embed") {
				return nil
			}
			if seen[p] {
				return nil
			}
			seen[p] = true
			info, err := d.Info()
			if err != nil || info.Size() < 100*1024*1024 {
				return nil
			}
			gb := float64(info.Size()) / (1024 * 1024 * 1024)
			out = append(out, GGUFModelInfo{
				Name: d.Name(),
				Path: p,
				Size: fmt.Sprintf("%.2f GB", gb),
			})
			return nil
		})
	}
	return out
}

func findQNNPython() string {
	var candidates []string
	if localApp := os.Getenv("LocalAppData"); localApp != "" {
		candidates = append(candidates,
			filepath.Join(localApp, "Programs", "Python", "Python312-arm64", "python.exe"),
			filepath.Join(localApp, "Programs", "Python", "Python312", "python.exe"),
		)
	}
	candidates = append(candidates, `C:\Program Files\Python312\python.exe`)
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("python"); err == nil && !strings.Contains(strings.ToLower(p), "windowsapps") {
		return p
	}
	return ""
}

var (
	hwMu       sync.RWMutex
	cachedHW   NPUHardwareInfo
	lastHWScan time.Time
)

func getAIConfig(db *gorm.DB) AIConfig {
	var cfg AIConfig
	if err := db.First(&cfg, 1).Error; err != nil {
		cfg = AIConfig{
			ID:          1,
			EndpointURL: "",
			ModelName:   "",
			SystemNote:  "คุณคือผู้ช่วย AI ประจำโรงพยาบาล ตอบเป็นภาษาไทยที่สุภาพ กระชับ และเข้าใจง่าย",
		}
		db.Create(&cfg)
	}
	return cfg
}

func scanNPUHardware(cfg AIConfig) NPUHardwareInfo {
	hwMu.RLock()
	if time.Since(lastHWScan) < 12*time.Second && cachedHW.DetectedAtUnix > 0 {
		res := cachedHW
		hwMu.RUnlock()
		return res
	}
	hwMu.RUnlock()

	info := NPUHardwareInfo{
		Arch:           runtime.GOARCH,
		DetectedAtUnix: time.Now().Unix(),
	}

	if runtime.GOOS == "windows" {
		winDir := os.Getenv("WINDIR")
		if winDir == "" {
			winDir = `C:\Windows`
		}
		if _, err := os.Stat(filepath.Join(winDir, "System32", "DirectML.dll")); err == nil {
			info.DirectMLReady = true
		}

		cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command",
			`Get-PnpDevice -Class ComputeAccelerator -ErrorAction SilentlyContinue | Select-Object -First 1 | ForEach-Object { "$($_.Status)|$($_.FriendlyName)" }`)
		hideCmdWindow(cmd)
		if out, err := cmd.Output(); err == nil {
			line := strings.TrimSpace(string(out))
			if parts := strings.SplitN(line, "|", 2); len(parts) == 2 {
				info.DeviceStatus = strings.TrimSpace(parts[0])
				info.DeviceName = strings.TrimSpace(parts[1])
			}
		}
		if _, err := exec.LookPath("foundry"); err == nil {
			info.FoundryCLI = true
		}
		if py := findQNNPython(); py != "" {
			info.PythonPath = py
			chk := exec.Command(py, "-c", "import onnxruntime_qnn; print('QNN_OK')")
			hideCmdWindow(chk)
			if out, err := chk.Output(); err == nil && strings.Contains(string(out), "QNN_OK") {
				info.QNNReady = true
			}
		}
		info.LlamaExe = findLlamaServer()
		llamaMu.Lock()
		info.LlamaRunning = llamaCmd != nil && llamaCmd.Process != nil
		llamaMu.Unlock()
		npuMu.Lock()
		info.NPULLMRunning = npuCmd != nil && npuCmd.Process != nil
		npuMu.Unlock()
		if qdir := findQNNModelDir(); qdir != "" {
			info.NPULLMModel = filepath.Base(qdir)
		}
	}

	if info.DeviceName == "" {
		info.DeviceName = "CPU / Standard Compute"
		info.DeviceStatus = "OK"
	}

	// Probe OpenAI-compatible local AI endpoints (Custom, llama.cpp, Foundry Local, Ollama, LM Studio)
	candidates := []struct {
		name string
		url  string
	}{
		{"Custom Endpoint", strings.TrimRight(strings.TrimSpace(cfg.EndpointURL), "/")},
		{"Hexagon NPU LLM", "http://127.0.0.1:8098"},
		{"llama.cpp Server", "http://127.0.0.1:8099"},
		{"llama.cpp Default", "http://127.0.0.1:8080"},
		{"Foundry Local (NPU)", "http://127.0.0.1:5272"},
		{"Foundry Local (Alt)", "http://127.0.0.1:5273"},
		{"Ollama Local", "http://127.0.0.1:11434"},
		{"LM Studio", "http://127.0.0.1:1234"},
	}

	hc := &http.Client{Timeout: 900 * time.Millisecond}
	for _, c := range candidates {
		if c.url == "" {
			continue
		}
		resp, err := hc.Get(c.url + "/v1/models")
		if err != nil {
			continue
		}
		var mResp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&mResp)
		_ = resp.Body.Close()
		if resp.StatusCode == 200 {
			info.ActiveEngine = c.name
			info.ActiveEndpoint = c.url
			if cfg.ModelName != "" {
				info.ActiveModel = cfg.ModelName
			} else if len(mResp.Data) > 0 {
				info.ActiveModel = mResp.Data[0].ID
			}
			break
		}
	}

	hwMu.Lock()
	cachedHW = info
	lastHWScan = time.Now()
	hwMu.Unlock()
	return info
}

func callLocalLLM(cfg AIConfig, hw NPUHardwareInfo, connector *blocksdk.ConnectorClient, mode, prompt string) (string, string, int64, error) {
	start := time.Now()
	if hw.ActiveEndpoint == "" {
		// Helpful offline fallback + extractive summary when no local LLM server is started yet
		summary := buildOfflineAssistResponse(mode, prompt, hw)
		return summary, "Offline Helper (No LLM Server Running)", time.Since(start).Milliseconds(), nil
	}

	sysPrompt := cfg.SystemNote
	if mode == "chat" && connector != nil {
		if raw, err := connector.Call("webchain", "ward_summary", nil); err == nil && len(raw) > 2 {
			sysPrompt += "\nข้อมูลล่าสุดจากระบบ WebChain ของโรงพยาบาล (ใช้ตอบคำถามเมื่อเกี่ยวข้อง): " + string(raw)
		}
	}
	switch mode {
	case "summarize_web":
		sysPrompt += " หน้าที่ของคุณคือสรุปข่าว/ประกาศของหน่วยงานพยาบาลให้กระชับเป็นข้อๆ อ่านง่าย"
	case "quiz_learn":
		sysPrompt += " หน้าที่ของคุณคือช่วยออกข้อสอบปรนัย 4 ตัวเลือก พร้อมเฉลยและคำอธิบายสั้นๆ สำหรับบุคลากรทางการแพทย์"
	case "analyze_doc":
		sysPrompt += " หน้าที่ของคุณคือสรุปสาระสำคัญ ขั้นตอนปฏิบัติ (SOP) และข้อควรระวังจากเอกสาร"
	}

	model := hw.ActiveModel
	if cfg.ModelName != "" {
		model = cfg.ModelName
	}
	if model == "" {
		model = "default"
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.4,
		"max_tokens":  900,
	})

	hc := &http.Client{Timeout: 60 * time.Second}
	resp, err := hc.Post(hw.ActiveEndpoint+"/v1/chat/completions", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", hw.ActiveEngine, time.Since(start).Milliseconds(), err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", hw.ActiveEngine, time.Since(start).Milliseconds(), fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Choices) == 0 {
		return "", hw.ActiveEngine, time.Since(start).Milliseconds(), fmt.Errorf("invalid response from %s", hw.ActiveEngine)
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), hw.ActiveEngine + " (" + model + ")", time.Since(start).Milliseconds(), nil
}

func buildOfflineAssistResponse(mode, prompt string, hw NPUHardwareInfo) string {
	lines := strings.Split(strings.TrimSpace(prompt), "\n")
	var nonEmpty []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}
	preview := prompt
	if len(nonEmpty) > 3 {
		preview = strings.Join(nonEmpty[:3], "\n• ")
	}
	return fmt.Sprintf(
		"🧠 ตรวจพบฮาร์ดแวร์ NPU: %s (%s)\n"+
			"⚠️ แต่ยังไม่พบ Local AI Model Server ที่กำลังเปิดใช้งาน (Foundry Local / Ollama)\n\n"+
			"วิธีเปิดใช้งาน NPU บนเครื่องนี้ใน 2 คำสั่ง (PowerShell):\n"+
			"1. winget install Microsoft.FoundryLocal\n"+
			"2. foundry model run qwen2.5-1.5b\n\n"+
			"เมื่อเปิดแล้ว กด 'รีเฟรชสถานะ NPU' ด้านบน AI จะประมวลผลผ่าน Hexagon NPU ทันทีครับ\n\n"+
			"--- ข้อความที่ส่งเข้ามา (%d บรรทัด) ---\n• %s",
		hw.DeviceName, hw.DeviceStatus, len(nonEmpty), preview,
	)
}

func main() {
	env := blocksdk.LoadRuntimeEnv("ai", "8084")
	db, err := blocksdk.OpenBlockDB(env.DBPath)
	if err != nil {
		log.Fatalf("[AI] DB open failed: %v", err)
	}
	if err := db.AutoMigrate(&AIConfig{}, &ChatLog{}); err != nil {
		log.Fatalf("[AI] AutoMigrate failed: %v", err)
	}

	connector := blocksdk.NewConnectorClient(env)
	app := fiber.New(fiber.Config{
		AppName: "AI NPU Block",
	})
	app.Use(recover.New())
	app.Use(blocksdk.UnifiedAuthMiddleware(env.BusSecret))

	blocksdk.RegisterStandardRoutes(app, blocksdk.StandardOptions{
		BlockID: env.BlockID,
		Version: "1.0.0",
		StatsProvider: func() blocksdk.BlockStats {
			var count int64
			db.Model(&ChatLog{}).Count(&count)
			cfg := getAIConfig(db)
			hw := scanNPUHardware(cfg)
			summary := fmt.Sprintf("%s • %d prompts", hw.DeviceName, count)
			if hw.ActiveEngine != "" {
				summary = fmt.Sprintf("NPU Ready (%s) • %d prompts", hw.ActiveEngine, count)
			}
			return blocksdk.BlockStats{
				BlockID: env.BlockID,
				Status:  "running",
				Summary: summary,
				Metrics: map[string]any{
					"prompts":       count,
					"npu_device":    hw.DeviceName,
					"active_engine": hw.ActiveEngine,
				},
			}
		},
		RPCHandlers: map[string]blocksdk.RPCHandler{
			"npu_status": func(c *fiber.Ctx, payload map[string]any) (any, error) {
				cfg := getAIConfig(db)
				return scanNPUHardware(cfg), nil
			},
			"ask": func(c *fiber.Ctx, payload map[string]any) (any, error) {
				cfg := getAIConfig(db)
				hw := scanNPUHardware(cfg)
				prompt, _ := payload["prompt"].(string)
				if prompt == "" {
					prompt, _ = payload["text"].(string)
				}
				reply, engine, ms, err := callLocalLLM(cfg, hw, connector, "chat", prompt)
				if err != nil {
					return nil, err
				}
				return map[string]any{"reply": reply, "engine": engine, "latency_ms": ms}, nil
			},
			"summarize": func(c *fiber.Ctx, payload map[string]any) (any, error) {
				cfg := getAIConfig(db)
				hw := scanNPUHardware(cfg)
				prompt, _ := payload["prompt"].(string)
				if prompt == "" {
					prompt, _ = payload["text"].(string)
				}
				reply, engine, ms, err := callLocalLLM(cfg, hw, connector, "summarize_web", prompt)
				if err != nil {
					return nil, err
				}
				return map[string]any{"reply": reply, "engine": engine, "latency_ms": ms}, nil
			},
		},
	})

	requireAIAdmin := func(c *fiber.Ctx) error {
		u := blocksdk.GetCoreUser(c)
		if u == nil {
			return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
		}
		if u.BlockRole != "admin" && u.BlockRole != "manager" && u.GlobalRole != "admin" && u.GlobalRole != "manager" && u.GlobalRole != "god" {
			return c.Status(403).JSON(fiber.Map{"error": "เฉพาะผู้จัดการ (Admin) เท่านั้นที่ปรับแต่งหรือควบคุมโมเดล AI ได้"})
		}
		return c.Next()
	}

	api := app.Group("/api")
	api.Get("/npu/status", func(c *fiber.Ctx) error {
		if c.Query("refresh") == "1" {
			hwMu.Lock()
			lastHWScan = time.Time{}
			hwMu.Unlock()
		}
		cfg := getAIConfig(db)
		hw := scanNPUHardware(cfg)
		return c.JSON(fiber.Map{"hardware": hw, "config": cfg, "user": blocksdk.GetCoreUser(c)})
	})

	api.Post("/npu/benchmark", requireAIAdmin, func(c *fiber.Ctx) error {
		cfg := getAIConfig(db)
		hw := scanNPUHardware(cfg)
		if !hw.QNNReady || hw.PythonPath == "" {
			return c.Status(400).JSON(fiber.Map{"error": "ยังไม่พบ onnxruntime-qnn สำหรับ Hexagon NPU"})
		}
		pyCode := `import json, time, numpy as np, onnx, onnxruntime as ort, onnxruntime_qnn
from onnx import helper, TensorProto
ort.register_execution_provider_library('QNNExecutionProvider', onnxruntime_qnn.get_library_path())
X = helper.make_tensor_value_info('X', TensorProto.FLOAT, [64, 1024])
W = helper.make_tensor('W', TensorProto.FLOAT, [1024, 1024], np.random.randn(1024, 1024).astype(np.float32).flatten().tolist())
Y = helper.make_tensor_value_info('Y', TensorProto.FLOAT, [64, 1024])
node = helper.make_node('MatMul', ['X', 'W'], ['Y'])
graph = helper.make_graph([node], 'npu_bench', [X], [Y], [W])
model = helper.make_model(graph, opset_imports=[helper.make_opsetid('', 17)])
sess = ort.InferenceSession(model.SerializeToString(), providers=[('QNNExecutionProvider', {'backend_path': onnxruntime_qnn.get_qnn_htp_path()})])
inp = np.random.randn(64, 1024).astype(np.float32)
sess.run(None, {'X': inp})
iters = 0
t0 = time.perf_counter()
while (time.perf_counter() - t0) < 4.0:
    sess.run(None, {'X': inp})
    iters += 1
dt = (time.perf_counter() - t0) * 1000.0
print(json.dumps({'ok': True, 'provider': 'QNNExecutionProvider (Hexagon HTP NPU)', 'iterations': iters, 'total_ms': round(dt, 2), 'avg_ms': round(dt/max(1,iters), 4)}))`
		cmd := exec.Command(hw.PythonPath, "-c", pyCode)
		hideCmdWindow(cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("NPU run failed: %v (%s)", err, string(out))})
		}
		c.Set("Content-Type", "application/json")
		return c.Send(bytes.TrimSpace(out))
	})

	api.Post("/npu/llm/start", requireAIAdmin, func(c *fiber.Ctx) error {
		startNPUServer(env.DataDir)
		hwMu.Lock()
		lastHWScan = time.Time{}
		hwMu.Unlock()
		return c.JSON(fiber.Map{"ok": true})
	})

	api.Post("/npu/llm/stop", requireAIAdmin, func(c *fiber.Ctx) error {
		npuMu.Lock()
		if npuCmd != nil && npuCmd.Process != nil {
			_ = npuCmd.Process.Kill()
			npuCmd = nil
		}
		npuMu.Unlock()
		hwMu.Lock()
		lastHWScan = time.Time{}
		hwMu.Unlock()
		return c.JSON(fiber.Map{"ok": true})
	})

	api.Post("/config", requireAIAdmin, func(c *fiber.Ctx) error {
		var body AIConfig
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
		}
		cfg := getAIConfig(db)
		cfg.EndpointURL = strings.TrimSpace(body.EndpointURL)
		cfg.ModelName = strings.TrimSpace(body.ModelName)
		if strings.TrimSpace(body.SystemNote) != "" {
			cfg.SystemNote = strings.TrimSpace(body.SystemNote)
		}
		db.Save(&cfg)
		hwMu.Lock()
		lastHWScan = time.Time{}
		hwMu.Unlock()
		return c.JSON(fiber.Map{"ok": true, "config": cfg})
	})

	api.Post("/chat", func(c *fiber.Ctx) error {
		var body struct {
			Mode   string `json:"mode"`
			Prompt string `json:"prompt"`
		}
		if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Prompt) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาใส่ข้อความ"})
		}
		cfg := getAIConfig(db)
		hw := scanNPUHardware(cfg)
		reply, engine, ms, err := callLocalLLM(cfg, hw, connector, body.Mode, body.Prompt)
		if err != nil {
			return c.Status(502).JSON(fiber.Map{"error": err.Error()})
		}
		entry := ChatLog{
			Mode:      body.Mode,
			Prompt:    body.Prompt,
			Response:  reply,
			Engine:    engine,
			LatencyMs: ms,
			CreatedAt: time.Now(),
		}
		db.Create(&entry)
		return c.JSON(fiber.Map{
			"reply":      reply,
			"engine":     engine,
			"latency_ms": ms,
			"hardware":   hw,
		})
	})

	api.Get("/history", func(c *fiber.Ctx) error {
		var logs []ChatLog
		db.Order("created_at desc").Limit(15).Find(&logs)
		return c.JSON(fiber.Map{"logs": logs})
	})

	api.Get("/llama/models", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"llama_exe": findLlamaServer(),
			"models":    scanGGUFModels(),
		})
	})

	api.Post("/llama/start", requireAIAdmin, func(c *fiber.Ctx) error {
		var body struct {
			ModelPath string `json:"model_path"`
		}
		if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.ModelPath) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาเลือกไฟล์โมเดล .gguf"})
		}
		exe := findLlamaServer()
		if exe == "" {
			return c.Status(400).JSON(fiber.Map{"error": "ไม่พบ llama-server.exe ในเครื่อง"})
		}
		if _, err := os.Stat(body.ModelPath); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "ไม่พบไฟล์โมเดล: " + body.ModelPath})
		}

		llamaMu.Lock()
		if llamaCmd != nil && llamaCmd.Process != nil {
			_ = llamaCmd.Process.Kill()
			llamaCmd = nil
		}
		cmd := exec.Command(exe, "-m", body.ModelPath, "--host", "127.0.0.1", "--port", "8099", "-c", "4096", "-ngl", "99")
		cmd.Dir = filepath.Dir(exe)
		hideCmdWindow(cmd)
		if err := cmd.Start(); err != nil {
			llamaMu.Unlock()
			return c.Status(500).JSON(fiber.Map{"error": "เปิด llama-server ไม่สำเร็จ: " + err.Error()})
		}
		llamaCmd = cmd
		llamaMu.Unlock()

		// Wait up to 20s for llama-server /v1/models to become ready
		hc := &http.Client{Timeout: 800 * time.Millisecond}
		ready := false
		for i := 0; i < 40; i++ {
			time.Sleep(500 * time.Millisecond)
			if resp, err := hc.Get("http://127.0.0.1:8099/v1/models"); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == 200 {
					ready = true
					break
				}
			}
		}
		hwMu.Lock()
		lastHWScan = time.Time{}
		hwMu.Unlock()
		return c.JSON(fiber.Map{"ok": true, "ready": ready, "endpoint": "http://127.0.0.1:8099"})
	})

	api.Post("/llama/stop", requireAIAdmin, func(c *fiber.Ctx) error {
		llamaMu.Lock()
		if llamaCmd != nil && llamaCmd.Process != nil {
			_ = llamaCmd.Process.Kill()
			llamaCmd = nil
		}
		llamaMu.Unlock()
		hwMu.Lock()
		lastHWScan = time.Time{}
		hwMu.Unlock()
		return c.JSON(fiber.Map{"ok": true})
	})

	distFS, err := fs.Sub(embedDist, "dist")
	if err != nil {
		log.Fatalf("Failed to load embedded dist/: %v", err)
	}
	app.Get("/", func(c *fiber.Ctx) error {
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

	addr := "127.0.0.1:" + env.Port
	go func() {
		if err := app.Listen(addr); err != nil {
			log.Fatalf("AI server stopped: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	llamaMu.Lock()
	if llamaCmd != nil && llamaCmd.Process != nil {
		_ = llamaCmd.Process.Kill()
	}
	llamaMu.Unlock()
	npuMu.Lock()
	if npuCmd != nil && npuCmd.Process != nil {
		_ = npuCmd.Process.Kill()
	}
	npuMu.Unlock()
	_ = app.Shutdown()
}
