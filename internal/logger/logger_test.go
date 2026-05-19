package logger_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"MRMI_Gateway/internal/logger"
)

func TestInit_JSONFormat(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger.Init("info", "json")
	logger.Info("test message", "pkg", "policy", "region_from", "RS")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if entry["ts"] == "" || entry["ts"] == nil {
		t.Errorf("expected 'ts' key in JSON output, got: %v", entry)
	}
	if entry["msg"] != "test message" {
		t.Errorf("expected msg='test message', got: %v", entry["msg"])
	}
	if entry["pkg"] != "policy" {
		t.Errorf("expected pkg='policy', got: %v", entry["pkg"])
	}
}

func TestInit_TextFormat(t *testing.T) {
	logger.Init("info", "text")
	// text handler should not panic and slog.Default should be updated
	h := slog.Default().Handler()
	if _, ok := h.(*slog.TextHandler); !ok {
		t.Errorf("expected slog.Default to use TextHandler after Init text")
	}
}

func TestInit_LevelFiltering(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger.Init("warn", "json")
	logger.Debug("should not appear")
	logger.Info("should not appear either")
	logger.Warn("this should appear")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	out := buf.String()
	if out == "" {
		t.Fatal("expected at least one log line at warn level")
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(out), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if entry["msg"] != "this should appear" {
		t.Errorf("unexpected msg: %v", entry["msg"])
	}
}
