package grpctransport

type Envelope struct {
	IdempotencyKey    string `json:"idempotency_key"`
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
