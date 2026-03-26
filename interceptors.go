package nekogrpc

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func authUnaryInterceptor(token string, metrics *Metrics) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := checkAuth(ctx, token); err != nil {
			metrics.authRejects.Add(1)
			return nil, err
		}
		metrics.unaryCalls.Add(1)
		return handler(ctx, req)
	}
}

func authStreamInterceptor(token string, metrics *Metrics) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := checkAuth(ss.Context(), token); err != nil {
			metrics.authRejects.Add(1)
			return err
		}
		metrics.streamCalls.Add(1)
		return handler(srv, ss)
	}
}

func latencyUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		_ = start
		return resp, err
	}
}

func latencyStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		_ = start
		return err
	}
}

func checkAuth(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get("authorization")
	if len(values) == 0 || values[0] != "Bearer "+token {
		return status.Error(codes.Unauthenticated, "invalid token")
	}
	return nil
}
