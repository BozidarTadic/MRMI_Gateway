package audit

import (
	"encoding/json"
	"os"
	"testing"

	"MRMI_Gateway/internal/config"
)

func TestVerifyFile_ValidChain(t *testing.T) {
	log := New()
	cfg := config.DefaultBalancedConfig()
	log.Append(cfg, DecisionAllow, "POLICY_ACCEPTED", 0, "RS", "RU")
	log.Append(cfg, DecisionDeny, "RECIPIENT_REGION_NOT_IN_ALLOW_LIST", 0, "RS", "US")

	path := t.TempDir() + "/log.json"
	if err := log.WriteJSON(path); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	rootHash, err := VerifyFile(path)
	if err != nil {
		t.Fatalf("VerifyFile: %v", err)
	}
	if rootHash == "" {
		t.Fatal("expected non-empty root hash")
	}
	if rootHash != log.RootHash() {
		t.Fatalf("root hash mismatch: file=%q log=%q", rootHash, log.RootHash())
	}
}

func TestVerifyFile_EmptyLog(t *testing.T) {
	log := New()
	path := t.TempDir() + "/empty.json"
	if err := log.WriteJSON(path); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	rootHash, err := VerifyFile(path)
	if err != nil {
		t.Fatalf("VerifyFile on empty log: %v", err)
	}
	if rootHash != zeroHash() {
		t.Fatalf("expected zero hash for empty log, got %q", rootHash)
	}
}

func TestVerifyFile_TamperedEntryDetected(t *testing.T) {
	log := New()
	cfg := config.DefaultBalancedConfig()
	log.Append(cfg, DecisionAllow, "POLICY_ACCEPTED", 0, "RS", "RU")

	path := t.TempDir() + "/tampered.json"
	if err := log.WriteJSON(path); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	// Tamper: change the decision in the JSON file.
	data, _ := os.ReadFile(path)
	var entries []Entry
	_ = json.Unmarshal(data, &entries)
	entries[0].Decision = DecisionDeny // mutate after writing
	raw, _ := json.Marshal(entries)
	_ = os.WriteFile(path, raw, 0644)

	if _, err := VerifyFile(path); err == nil {
		t.Fatal("expected VerifyFile to detect tamper, got nil error")
	}
}
