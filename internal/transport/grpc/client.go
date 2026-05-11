package grpctransport

import (
	"context"
	"crypto/tls"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"MRMI_Gateway/internal/discovery"
)

type Client struct {
	conn *grpc.ClientConn
}

// Dial connects to a gateway gRPC server at target.
// Pass a non-nil tlsCfg to use mTLS; pass nil for insecure (development only).
func Dial(ctx context.Context, target string, tlsCfg *tls.Config) (*Client, error) {
	var creds credentials.TransportCredentials
	if tlsCfg != nil {
		creds = credentials.NewTLS(tlsCfg)
	} else {
		creds = insecure.NewCredentials()
	}

	conn, err := grpc.DialContext( //nolint:staticcheck // migrate to grpc.NewClient in Sprint 2
		ctx,
		target,
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(jsonCodec{})),
	)
	if err != nil {
		return nil, err
	}

	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) SendEnvelope(ctx context.Context, request *SendEnvelopeRequest) (*SendEnvelopeResponse, error) {
	response := new(SendEnvelopeResponse)
	if err := c.conn.Invoke(ctx, "/mrmi.v1.GatewayService/SendEnvelope", request, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) GetNodeInfo(ctx context.Context, request *GetNodeInfoRequest) (*GetNodeInfoResponse, error) {
	response := new(GetNodeInfoResponse)
	if err := c.conn.Invoke(ctx, "/mrmi.v1.GatewayService/GetNodeInfo", request, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) ShareRootHash(ctx context.Context, request *RootHashMessage) (*RootHashAck, error) {
	response := new(RootHashAck)
	if err := c.conn.Invoke(ctx, "/mrmi.v1.GatewayService/ShareRootHash", request, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) BroadcastDiscovery(ctx context.Context, request *DiscoveryRequest) (*DiscoveryResponse, error) {
	response := new(DiscoveryResponse)
	if err := c.conn.Invoke(ctx, "/mrmi.v1.GatewayService/BroadcastDiscovery", request, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) Connect(ctx context.Context, request *ConnectRequest) (*ConnectAck, error) {
	response := new(ConnectAck)
	if err := c.conn.Invoke(ctx, "/mrmi.v1.GatewayService/Connect", request, response); err != nil {
		return nil, err
	}
	return response, nil
}

// AsDiscoveryClient wraps this Client so it satisfies discovery.PeerClient,
// converting between grpc transport types and discovery domain types.
func (c *Client) AsDiscoveryClient() discovery.PeerClient {
	return &discoveryClientAdapter{c}
}

type discoveryClientAdapter struct {
	c *Client
}

func (a *discoveryClientAdapter) BroadcastDiscovery(ctx context.Context, req *discovery.Request) (*discovery.Response, error) {
	resp, err := a.c.BroadcastDiscovery(ctx, &DiscoveryRequest{
		QueryHash:    req.QueryHash,
		QueryType:    req.QueryType,
		OriginNodeID: req.OriginNodeID,
		OriginAppID:  req.OriginAppID,
		HopLimit:     req.HopLimit,
		RequestID:    req.RequestID,
		Timestamp:    req.Timestamp,
	})
	if err != nil {
		return nil, err
	}
	return &discovery.Response{
		NodeID:       resp.NodeID,
		AppID:        resp.AppID,
		OpaqueToken:  resp.OpaqueToken,
		DisplayHint:  resp.DisplayHint,
		MatchType:    resp.MatchType,
		TokenExpires: resp.TokenExpires,
	}, nil
}

func (a *discoveryClientAdapter) Close() error {
	return a.c.Close()
}
