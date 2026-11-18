package blocksdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ConnectorClient allows a block to call other blocks and publish events via CoreChain Connector Bus.
type ConnectorClient struct {
	BlockID    string
	BusURL     string
	BusSecret  string
	httpClient *http.Client
}

// NewConnectorClient creates a ConnectorClient for a block.
func NewConnectorClient(env RuntimeEnv) *ConnectorClient {
	return &ConnectorClient{
		BlockID:   env.BlockID,
		BusURL:    env.BusURL,
		BusSecret: env.BusSecret,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

// Call invokes an RPC action on another block through CoreChain Connector Bus.
func (cc *ConnectorClient) Call(targetBlock, action string, payload map[string]any) (json.RawMessage, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api/connector/call/%s/%s", cc.BusURL, targetBlock, action)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Core-Bus-Secret", cc.BusSecret)
	req.Header.Set("X-Core-Sender-Block", cc.BlockID)

	resp, err := cc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("connector call %s.%s failed (%d): %s", targetBlock, action, resp.StatusCode, string(body))
	}
	return body, nil
}

// EmitEvent publishes an event to the CoreChain Pub/Sub Event Bus.
func (cc *ConnectorClient) EmitEvent(topic string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	bodyMap := map[string]any{
		"topic":        topic,
		"sender_block": cc.BlockID,
		"payload":      payload,
	}
	raw, err := json.Marshal(bodyMap)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/connector/events", cc.BusURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Core-Bus-Secret", cc.BusSecret)
	req.Header.Set("X-Core-Sender-Block", cc.BlockID)

	resp, err := cc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("emit event %s failed (%d): %s", topic, resp.StatusCode, string(b))
	}
	return nil
}
