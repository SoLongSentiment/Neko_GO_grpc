package nekogrpc

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

func BenchmarkNekoGRPC(b *testing.B) {
	listener := bufconn.Listen(1024 * 1024)
	server := NewServer("bench-token")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx, listener) }()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		b.Fatalf("grpc dial: %v", err)
	}
	defer conn.Close()

	client := NewClient(conn)
	authCtx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer bench-token")

	b.Run("unary_put", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := client.Put(authCtx, MustStruct(map[string]any{"key": "bench", "value": "hello"})); err != nil {
				b.Fatalf("put: %v", err)
			}
		}
	})

	b.Run("chat_roundtrip", func(b *testing.B) {
		stream, err := client.Chat(authCtx)
		if err != nil {
			b.Fatalf("chat: %v", err)
		}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := stream.Send(MustStruct(map[string]any{"room": "bench", "user": "alice", "message": "ping"})); err != nil {
				b.Fatalf("send: %v", err)
			}
			if _, err := stream.Recv(); err != nil {
				b.Fatalf("recv: %v", err)
			}
		}
	})
}
