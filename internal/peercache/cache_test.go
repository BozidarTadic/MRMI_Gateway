package peercache

import "testing"

func TestCache_StoreAndAll(t *testing.T) {
	c := New()

	c.Store("ru-node-01", "sha256:aabbcc", 1000)
	c.Store("by-node-01", "sha256:ddeeff", 2000)

	all := c.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
	if all["ru-node-01"].RootHash != "sha256:aabbcc" {
		t.Errorf("ru-node-01 root hash mismatch")
	}
	if all["by-node-01"].Timestamp != 2000 {
		t.Errorf("by-node-01 timestamp mismatch")
	}
}

func TestCache_UpdateOverwrites(t *testing.T) {
	c := New()
	c.Store("ru-node-01", "sha256:old", 1)
	c.Store("ru-node-01", "sha256:new", 2)

	all := c.All()
	if all["ru-node-01"].RootHash != "sha256:new" {
		t.Errorf("expected updated root hash")
	}
	if len(all) != 1 {
		t.Errorf("expected 1 entry after overwrite, got %d", len(all))
	}
}
