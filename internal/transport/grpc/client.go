package grpctransport

import (
	"context"
	"crypto/tls"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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
