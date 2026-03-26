package nekogrpc

import (
	"context"
	"net"

	"google.golang.org/grpc"
)

type Server struct {
	service *Service
	token   string
	grpc    *grpc.Server
}

func NewServer(token string) *Server {
	service := NewService()
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			authUnaryInterceptor(token, service.metrics),
			latencyUnaryInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			authStreamInterceptor(token, service.metrics),
			latencyStreamInterceptor(),
		),
	)
	RegisterNekoService(server, service)
	return &Server{service: service, token: token, grpc: server}
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		s.grpc.GracefulStop()
	}()
	return s.grpc.Serve(ln)
}

func (s *Server) GRPC() *grpc.Server { return s.grpc }
func (s *Server) Service() *Service  { return s.service }
