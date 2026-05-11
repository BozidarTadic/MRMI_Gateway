package peerdiscovery

import (
	"testing"
	"time"
)

func TestAnnounceAndKnown(t *testing.T) {
	r := New()
	r.Announce(PeerInfo{NodeID: "n1", Addr: ":7001", NodeScope: "regional", Region: "RS"})
	r.Announce(PeerInfo{NodeID: "n2", Addr: ":7002", NodeScope: "regional", Region: "RU"})
	peers := r.Known()
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
}

func TestEvict(t *testing.T) {
	r := New()
	r.Announce(PeerInfo{NodeID: "n1", Addr: ":7001"})
	r.Evict("n1")
	if len(r.Known()) != 0 {
		t.Fatal("expected 0 peers after evict")
	}
}

func TestEvictStale(t *testing.T) {
	r := New()
	r.Announce(PeerInfo{NodeID: "fresh", Addr: ":7001"})
	// manually set an old last-seen
	r.mu.Lock()
	old := r.peers["fresh"]
	old.LastSeen = time.Now().Add(-10 * time.Minute)
	r.peers["fresh"] = old
	r.mu.Unlock()

	r.Announce(PeerInfo{NodeID: "new", Addr: ":7002"})
	r.EvictStale(5 * time.Minute)
	peers := r.Known()
	if len(peers) != 1 || peers[0].NodeID != "new" {
		t.Fatalf("expected only 'new' peer after evict-stale, got %v", peers)
	}
}

func TestAnnounce_RefreshesLastSeen(t *testing.T) {
	r := New()
	r.Announce(PeerInfo{NodeID: "n1", Addr: ":7001"})
	first := r.Known()[0].LastSeen
	time.Sleep(2 * time.Millisecond)
	r.Announce(PeerInfo{NodeID: "n1", Addr: ":7001"})
	second := r.Known()[0].LastSeen
	if !second.After(first) {
		t.Fatal("expected LastSeen to be refreshed on re-announce")
	}
}
