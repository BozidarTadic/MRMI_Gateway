package server

import (
	"encoding/json"
	"net/http"
	"time"

	"MRMI_Gateway/internal/audit"
	"MRMI_Gateway/internal/config"
	"MRMI_Gateway/internal/policy"
	"MRMI_Gateway/internal/version"
)

type HTTPServer struct {
	*http.Server
}

type auditResponse struct {
	Version       int    `json:"version"`
	AppVersion    string `json:"app_version"`
	ADRVersion    string `json:"adr_version"`
	Timestamp     int64  `json:"timestamp"`
	RootHash      string `json:"root_hash"`
	NodeID        string `json:"node_id"`
	ApplicableLaw string `json:"applicable_law"`
	Signature     string `json:"signature"`
}

func NewHTTPServer(cfg config.Config, engine *policy.Engine, auditLog *audit.Log) *HTTPServer {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if engine == nil {
			http.Error(w, "policy engine not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	mux.HandleFunc("/.well-known/mrmi-audit", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rootHash := "sha256:BOOTSTRAP_PLACEHOLDER"
		if auditLog != nil {
			rootHash = auditLog.RootHash()
		}
		_ = json.NewEncoder(w).Encode(auditResponse{
			Version:       1,
			AppVersion:    version.App,
			ADRVersion:    version.ADR,
			Timestamp:     time.Now().Unix(),
			RootHash:      rootHash,
			NodeID:        cfg.Node.NodeID,
			ApplicableLaw: cfg.Node.ApplicableLaw,
			Signature:     cfg.Node.SignedBy,
		})
	})

	return &HTTPServer{
		Server: &http.Server{
			Addr:              cfg.Network.HTTPListenAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}
