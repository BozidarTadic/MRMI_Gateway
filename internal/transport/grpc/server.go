package grpctransport

import (
	"context"
	"crypto/tls"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

type Server struct {
	server   *grpc.Server
	listener net.Listener
}

// NewServer creates a gRPC server on listenAddr and registers svc.
// Pass a non-nil tlsCfg to enable TLS; pass nil for insecure (development only).
func NewServer(listenAddr string, svc GatewayService, tlsCfg *tls.Config) (*Server, error) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	encoding.RegisterCodec(jsonCodec{})

	var opts []grpc.ServerOption
	if tlsCfg != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	} else {
		opts = append(opts, grpc.Creds(insecure.NewCredentials()))
	}
	opts = append(opts, grpc.ForceServerCodec(jsonCodec{}))

	server := grpc.NewServer(opts...)
	RegisterGatewayService(server, svc)

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
