package grpctransport

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GatewayService interface {
	SendEnvelope(context.Context, *SendEnvelopeRequest) (*SendEnvelopeResponse, error)
	GetNodeInfo(context.Context, *GetNodeInfoRequest) (*GetNodeInfoResponse, error)
}

func RegisterGatewayService(server grpc.ServiceRegistrar, svc GatewayService) {
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "mrmi.v1.GatewayService",
		HandlerType: (*GatewayService)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "SendEnvelope",
				Handler:    sendEnvelopeHandler(svc),
			},
			{
				MethodName: "GetNodeInfo",
				Handler:    getNodeInfoHandler(svc),
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "proto/mrmi/v1/contracts.proto",
	}, svc)
}

func sendEnvelopeHandler(svc GatewayService) grpc.MethodHandler {
	return func(service any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		request := new(SendEnvelopeRequest)
		if err := decode(request); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "decode send envelope request: %v", err)
		}

		if interceptor == nil {
			return svc.SendEnvelope(ctx, request)
		}

		info := &grpc.UnaryServerInfo{
			Server:     service,
			FullMethod: "/mrmi.v1.GatewayService/SendEnvelope",
		}
		handler := func(ctx context.Context, req any) (any, error) {
			typed, ok := req.(*SendEnvelopeRequest)
			if !ok {
				return nil, status.Error(codes.Internal, "unexpected request type")
			}
			return svc.SendEnvelope(ctx, typed)
		}
		return interceptor(ctx, request, info, handler)
	}
}

func getNodeInfoHandler(svc GatewayService) grpc.MethodHandler {
	return func(service any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		request := new(GetNodeInfoRequest)
		if err := decode(request); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "decode get node info request: %v", err)
		}

		if interceptor == nil {
			return svc.GetNodeInfo(ctx, request)
		}

		info := &grpc.UnaryServerInfo{
			Server:     service,
			FullMethod: "/mrmi.v1.GatewayService/GetNodeInfo",
		}
		handler := func(ctx context.Context, req any) (any, error) {
			typed, ok := req.(*GetNodeInfoRequest)
			if !ok {
				return nil, status.Error(codes.Internal, "unexpected request type")
			}
			return svc.GetNodeInfo(ctx, typed)
		}
		return interceptor(ctx, request, info, handler)
	}
}
