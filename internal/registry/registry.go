package registry

import (
	"strings"
	"sync"
	"time"

	"MRMI_Gateway/internal/config"

	"github.com/google/uuid"
)

// DiscoveryResult is returned for each matching user.
type DiscoveryResult struct {
	NodeID       string `json:"node_id"`
	AppID        string `json:"app_id"`
	UserID       string `json:"user_id"`
	DisplayHint  string `json:"display_hint"`
	Region       string `json:"region"`
	OpaqueToken  string `json:"opaque_token"`
	TokenExpires int64  `json:"token_expires"`
}

// ConnectResult is the response to a ConnectRequest.
type ConnectResult struct {
	Status    string `json:"status"` // "ACCEPTED" | "PENDING" | "DENIED"
	SessionID string `json:"session_id,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type tokenEntry struct {
	appID   string
	userID  string
	expires time.Time
}

// Registry holds the node's registered app users and issued opaque tokens.
type Registry struct {
	cfg    config.Config
	mu     sync.Mutex
	tokens map[string]tokenEntry // opaque_token → entry
}

const tokenTTL = 5 * time.Minute

func New(cfg config.Config) *Registry {
	return &Registry{
		cfg:    cfg,
		tokens: make(map[string]tokenEntry),
	}
}

// Discover searches registered users by display_hint or app_id.
// queryType: "display_hint" (default) or "app_id"
func (r *Registry) Discover(query, queryType string) []DiscoveryResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	q := strings.ToLower(query)
	var results []DiscoveryResult

	for appID, app := range r.cfg.Apps {
		for userID, u := range app.Users {
			var match bool
			switch queryType {
			case "app_id":
				match = strings.EqualFold(appID, query)
			default:
				match = strings.Contains(strings.ToLower(u.DisplayHint), q)
			}
			if !match {
				continue
			}
			token := uuid.NewString()
			expires := now.Add(tokenTTL)
			r.tokens[token] = tokenEntry{appID: appID, userID: userID, expires: expires}
			results = append(results, DiscoveryResult{
				NodeID:       r.cfg.Node.NodeID,
				AppID:        appID,
				UserID:       userID,
				DisplayHint:  u.DisplayHint,
				Region:       u.Region,
				OpaqueToken:  token,
				TokenExpires: expires.Unix(),
			})
		}
	}
	return results
}

// Connect processes a connect request using the opaque token from a previous Discover call.
// requesterRegion is the region of the caller.
func (r *Registry) Connect(opaqueToken, requesterID, requesterRegion string) ConnectResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.tokens[opaqueToken]
	if !ok || time.Now().After(entry.expires) {
		delete(r.tokens, opaqueToken)
		return ConnectResult{Status: "DENIED"}
	}
	delete(r.tokens, opaqueToken) // one-time use

	app, exists := r.cfg.Apps[entry.appID]
	if !exists {
		return ConnectResult{Status: "DENIED"}
	}

	autoAccept := app.AutoAccept
	if autoAccept == "" {
		autoAccept = "manual"
	}

	var status string
	switch autoAccept {
	case "auto_all":
		status = "ACCEPTED"
	case "auto_mutual":
		// Accept when the requester's region is in the allow_to list.
		allowed := false
		for _, region := range r.cfg.Policy.Outbound.AllowTo {
			if strings.EqualFold(region, requesterRegion) {
				allowed = true
				break
			}
		}
		if allowed {
			status = "ACCEPTED"
		} else {
			status = "PENDING"
		}
	case "auto_whitelist":
		// Accept only if requester is a known user in the same app.
		_, known := app.Users[requesterID]
		if known {
			status = "ACCEPTED"
		} else {
			status = "PENDING"
		}
	default: // "manual"
		status = "PENDING"
	}

	sessionID := uuid.NewString()
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	return ConnectResult{Status: status, SessionID: sessionID, ExpiresAt: expiresAt}
}
