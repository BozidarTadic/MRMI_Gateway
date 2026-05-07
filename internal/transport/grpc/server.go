package grpctransport

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

type Server struct {
	server   *grpc.Server
	listener net.Listener
}

func NewServer(listenAddr string, gateway GatewayService) (*Server, error) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	encoding.RegisterCodec(jsonCodec{})

	server := grpc.NewServer(grpc.ForceServerCodec(jsonCodec{}))
	RegisterGatewayService(server, gateway)

	return &Server{
		server:   server,
		listener: listener,
	}, nil
}

func (s *Server) Serve() error {
	return s.server.Serve(s.listener)
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *Server) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(done)
	}()

	select {
	case <-ctx.Done():
		s.server.Stop()
		return ctx.Err()
	case <-done:
		return nil
	}
}
