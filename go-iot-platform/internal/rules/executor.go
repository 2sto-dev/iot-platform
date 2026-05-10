package rules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var templateRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// RenderTemplate replaces {{field}} placeholders using ctx values.
func RenderTemplate(tmpl string, ctx map[string]interface{}) string {
	return templateRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		key := strings.TrimSpace(match[2 : len(match)-2])
		if v, ok := ctx[key]; ok {
			return fmt.Sprintf("%v", v)
		}
		// Try nested field extraction
		if m, ok := ctx["_payload"].(map[string]interface{}); ok {
			if v := ExtractField(m, key); v != nil {
				return fmt.Sprintf("%v", v)
			}
		}
		return match
	})
}

// Executor carries shared clients for action execution.
type Executor struct {
	mqttPub    mqtt.Client
	djangoBase string
	svcUser    string
	svcPass    string
	httpClient *http.Client
}

func NewExecutor(mqttPub mqtt.Client, djangoBase, svcUser, svcPass string) *Executor {
	return &Executor{
		mqttPub:    mqttPub,
		djangoBase: djangoBase,
		svcUser:    svcUser,
		svcPass:    svcPass,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Execute runs all actions for a rule and returns a summary of results.
func (e *Executor) Execute(ctx context.Context, rule Rule, msgCtx MessageContext, execID int64) []map[string]interface{} {
	// Build template context
	tplCtx := map[string]interface{}{
		"serial":    msgCtx.Serial,
		"tenant_id": msgCtx.TenantID,
		"stream":    msgCtx.Stream,
		"ts":        time.Now().UTC().Format(time.RFC3339),
		"_payload":  msgCtx.Payload,
	}
	// Flatten top-level payload fields into template context
	for k, v := range msgCtx.Payload {
		tplCtx[k] = v
	}

	results := make([]map[string]interface{}, 0, len(rule.Actions))
	for _, action := range rule.Actions {
		result := e.executeAction(ctx, action, msgCtx, tplCtx, rule, execID)
		results = append(results, result)
	}
	return results
}

func (e *Executor) executeAction(ctx context.Context, action Action, msgCtx MessageContext, tplCtx map[string]interface{}, rule Rule, execID int64) map[string]interface{} {
	switch action.Type {
	case "downlink":
		return e.execDownlink(ctx, action, msgCtx, tplCtx)
	case "notify":
		return e.execNotify(ctx, action, tplCtx, execID)
	case "webhook":
		return e.execWebhook(ctx, action, tplCtx)
	case "set_shadow":
		return e.execSetShadow(ctx, action, msgCtx)
	default:
		return map[string]interface{}{"type": action.Type, "error": "unknown action type"}
	}
}

func (e *Executor) execDownlink(_ context.Context, action Action, msgCtx MessageContext, tplCtx map[string]interface{}) map[string]interface{} {
	target := action.TargetSerial
	if target == "" || target == "{{serial}}" {
		target = msgCtx.Serial
	} else {
		target = RenderTemplate(target, tplCtx)
	}

	payload := map[string]interface{}{
		"action":  action.ActionName,
		"payload": action.Payload,
		"source":  "rule_engine",
	}
	data, _ := json.Marshal(payload)
	topic := fmt.Sprintf("tenants/%d/devices/%s/down/cmd", msgCtx.TenantID, target)

	tok := e.mqttPub.Publish(topic, 1, false, data)
	if tok.Wait() && tok.Error() != nil {
		log.Printf("rule executor: downlink publish failed: %v", tok.Error())
		return map[string]interface{}{"type": "downlink", "error": tok.Error().Error()}
	}
	return map[string]interface{}{"type": "downlink", "topic": topic, "action": action.ActionName}
}

func (e *Executor) execNotify(ctx context.Context, action Action, tplCtx map[string]interface{}, execID int64) map[string]interface{} {
	body := map[string]interface{}{
		"channel_id":        action.ChannelID,
		"title":             RenderTemplate(action.Title, tplCtx),
		"body":              RenderTemplate(action.Body, tplCtx),
		"rule_execution_id": execID,
		"context":           tplCtx,
	}
	err := e.djangoPost(ctx, "/api/internal/notifications/trigger/", body)
	if err != nil {
		log.Printf("rule executor: notify failed: %v", err)
		return map[string]interface{}{"type": "notify", "channel_id": action.ChannelID, "error": err.Error()}
	}
	return map[string]interface{}{"type": "notify", "channel_id": action.ChannelID, "ok": true}
}

func (e *Executor) execWebhook(_ context.Context, action Action, tplCtx map[string]interface{}) map[string]interface{} {
	method := action.Method
	if method == "" {
		method = "POST"
	}
	url := RenderTemplate(action.URL, tplCtx)

	var bodyBytes []byte
	if action.BodyTemplate != "" {
		bodyBytes = []byte(RenderTemplate(action.BodyTemplate, tplCtx))
	} else {
		defaultBody := map[string]interface{}{
			"serial":    tplCtx["serial"],
			"tenant_id": tplCtx["tenant_id"],
			"stream":    tplCtx["stream"],
			"ts":        tplCtx["ts"],
		}
		bodyBytes, _ = json.Marshal(defaultBody)
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return map[string]interface{}{"type": "webhook", "error": err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range action.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Printf("rule executor: webhook failed: %v", err)
		return map[string]interface{}{"type": "webhook", "url": url, "error": err.Error()}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return map[string]interface{}{"type": "webhook", "url": url, "status": resp.StatusCode}
}

func (e *Executor) execSetShadow(ctx context.Context, action Action, msgCtx MessageContext) map[string]interface{} {
	body := map[string]interface{}{"desired": action.Desired}
	// Uses the shadow/reported endpoint with serial lookup
	path := fmt.Sprintf("/api/shadow/reported/?serial=%s", msgCtx.Serial)
	// Actually update desired state — call PATCH /api/devices/{serial}/shadow/
	// We use the by-serial endpoint
	err := e.djangoPatch(ctx, path, map[string]interface{}{"desired": action.Desired})
	if err != nil {
		_ = body
		log.Printf("rule executor: set_shadow failed: %v", err)
		return map[string]interface{}{"type": "set_shadow", "error": err.Error()}
	}
	return map[string]interface{}{"type": "set_shadow", "desired": action.Desired}
}

// LogExecution calls Django to record a rule execution.
func (e *Executor) LogExecution(ctx context.Context, body map[string]interface{}) error {
	return e.djangoPost(ctx, "/api/internal/rules/log/", body)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (e *Executor) djangoPost(ctx context.Context, path string, body interface{}) error {
	return e.djangoRequest(ctx, "POST", path, body)
}

func (e *Executor) djangoPatch(ctx context.Context, path string, body interface{}) error {
	return e.djangoRequest(ctx, "PATCH", path, body)
}

func (e *Executor) djangoRequest(ctx context.Context, method, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, e.djangoBase+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(e.svcUser, e.svcPass)
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("django returned %d", resp.StatusCode)
	}
	return nil
}
