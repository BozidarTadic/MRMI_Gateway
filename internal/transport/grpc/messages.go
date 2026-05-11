package grpctransport

type Envelope struct {
	IdempotencyKey    string `json:"idempotency_key"`
	SenderNodeID      string `json:"sender_node_id,omitempty"`
	SenderIdentity    []byte `json:"sender_identity,omitempty"`
	RecipientIdentity []byte `json:"recipient_identity,omitempty"`
	SenderRegion      string `json:"sender_region"`
	RecipientRegion   string `json:"recipient_region"`
	TrustTier         uint32 `json:"trust_tier"`
	SequenceNumber    uint64 `json:"sequence_number"`
	Payload           []byte `json:"payload,omitempty"`
	PaddedTo          uint32 `json:"padded_to"`
	Timestamp         int64  `json:"timestamp"`
	Signature         []byte `json:"signature,omitempty"`
	IsDummy           bool   `json:"is_dummy,omitempty"`
}

type SendEnvelopeRequest struct {
	Envelope Envelope `json:"envelope"`
}

type SendEnvelopeResponse struct {
	Decision          string `json:"decision"`
	Reason            string `json:"reason"`
	Profile           string `json:"profile"`
	NodeID            string `json:"node_id"`
	AuditRootHash     string `json:"audit_root_hash"`
	PeerAuditRootHash string `json:"peer_audit_root_hash,omitempty"`
}

type GetNodeInfoRequest struct{}

type GetNodeInfoResponse struct {
	NodeID        string `json:"node_id"`
	NodeScope     string `json:"node_scope"`
	Region        string `json:"region"`
	ApplicableLaw string `json:"applicable_law"`
	Profile       string `json:"profile"`
}

type RootHashMessage struct {
	NodeID    string `json:"node_id"`
	RootHash  string `json:"root_hash"`
	Timestamp int64  `json:"timestamp"`
	Signature []byte `json:"signature,omitempty"`
}

type RootHashAck struct {
	Accepted bool `json:"accepted"`
}

type DiscoveryRequest struct {
	QueryHash    string `json:"query_hash"`
	QueryType    string `json:"query_type"`
	OriginNodeID string `json:"origin_node_id"`
	OriginAppID  string `json:"origin_app_id"`
	HopLimit     uint32 `json:"hop_limit"`
	RequestID    string `json:"request_id"`
	Timestamp    int64  `json:"timestamp"`
}

type DiscoveryResponse struct {
	NodeID       string `json:"node_id"`
	AppID        string `json:"app_id"`
	OpaqueToken  string `json:"opaque_token"`
	DisplayHint  string `json:"display_hint"`
	MatchType    string `json:"match_type"`
	TokenExpires int64  `json:"token_expires"`
}

type ConnectRequest struct {
	OpaqueToken  string `json:"opaque_token"`
	OriginNodeID string `json:"origin_node_id"`
	OriginAppID  string `json:"origin_app_id"`
	RequestID    string `json:"request_id"`
}

type ConnectAck struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type PeerEntry struct {
	NodeID    string `json:"node_id"`
	Addr      string `json:"addr"`
	NodeScope string `json:"node_scope"`
	Region    string `json:"region"`
	LastSeen  int64  `json:"last_seen"` // unix seconds
}

type PeerListRequest struct {
	SenderNodeID string      `json:"sender_node_id"`
	KnownPeers   []PeerEntry `json:"known_peers"`
}

type PeerListResponse struct {
	Peers []PeerEntry `json:"peers"`
}
