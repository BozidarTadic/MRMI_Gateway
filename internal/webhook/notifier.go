package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/core"
)

// payload is the JSON body POSTed to the app webhook URL.
type payload struct {
	NodeID          string `json:"node_id"`
	AppID           string `json:"app_id"`
	IdempotencyKey  string `json:"idempotency_key"`
	SenderRegion    string `json:"sender_region"`
	RecipientRegion string `json:"recipient_region"`
	Timestamp       int64  `json:"timestamp"`
}

// Notifier delivers best-effort webhook notifications to registered apps.
type Notifier struct {
	cfg config.Config
}

func New(cfg config.Config) *Notifier {
	return &Notifier{cfg: cfg}
}

// NotifyAll fires a notification to every app whose webhook_url is configured.
// Called after an ALLOW decision; non-blocking — errors are logged and discarded.
func (n *Notifier) NotifyAll(ctx context.Context, env core.Envelope) {
	for appID, app := range n.cfg.Apps {
		if app.WebhookURL == "" {
			continue
		}
		go func(id string, a config.AppConfig) {
			if err := n.notify(ctx, id, a, env); err != nil {
				log.Printf("[webhook] app %s: %v", id, err)
			}
		}(appID, app)
	}
}

func (n *Notifier) notify(ctx context.Context, appID string, app config.AppConfig, env core.Envelope) error {
	p := payload{
		NodeID:          n.cfg.Node.NodeID,
		AppID:           appID,
		IdempotencyKey:  env.IdempotencyKey,
		SenderRegion:    env.SenderRegion,
		RecipientRegion: env.RecipientRegion,
		Timestamp:       env.Timestamp,
	}
	body, _ := json.Marshal(p)

	timeout := time.Duration(app.WebhookTimeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, app.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if app.WebhookSecret != "" {
		req.Header.Set("X-MRMI-Signature", "sha256="+sign(body, app.WebhookSecret))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	resp.Body.Close()

	// Retry once on 5xx.
	if resp.StatusCode >= 500 {
		req2, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, app.WebhookURL, bytes.NewReader(body))
		req2.Header = req.Header.Clone()
		resp2, err := http.DefaultClient.Do(req2)
		if err != nil {
			return fmt.Errorf("retry post: %w", err)
		}
		resp2.Body.Close()
	}
	return nil
}

// sign returns the hex-encoded HMAC-SHA256 of body using secret.
func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks that the X-MRMI-Signature header matches the body.
// Exported for use in tests and app-side verification.
func VerifySignature(body []byte, secret, header string) bool {
	expected := "sha256=" + sign(body, secret)
	return hmac.Equal([]byte(expected), []byte(header))
}
