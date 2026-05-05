package grpctransport

import (
	"context"

	"google.golang.org/grpc"
)

type Client struct {
	conn *grpc.ClientConn
}

func Dial(ctx context.Context, target string) (*Client, error) {
	conn, err := grpc.DialContext(
		ctx,
		target,
		grpc.WithInsecure(),
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
