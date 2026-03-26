package nekogrpc

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func newTestClient(tb testing.TB) (NekoServiceClient, context.Context, func()) {
	tb.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := NewServer("test-token")
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = server.Serve(ctx, listener) }()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		tb.Fatalf("grpc dial: %v", err)
	}
	authCtx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer test-token")
	cleanup := func() {
		_ = conn.Close()
		cancel()
		_ = listener.Close()
	}
	return NewClient(conn), authCtx, cleanup
}

func TestPutGet(t *testing.T) {
	client, authCtx, cleanup := newTestClient(t)
	defer cleanup()

	if _, err := client.Put(authCtx, MustStruct(map[string]any{"key": "one", "value": "hello"})); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := client.Get(authCtx, MustStruct(map[string]any{"key": "one"}))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if structString(got, "key") != "one" {
		t.Fatalf("unexpected key: %v", got)
	}
}

func TestWatchReceivesUpdates(t *testing.T) {
	client, authCtx, cleanup := newTestClient(t)
	defer cleanup()

	watch, err := client.Watch(authCtx, MustStruct(map[string]any{"key": "watched"}))
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if _, err := client.Put(authCtx, MustStruct(map[string]any{"key": "watched", "value": "v1"})); err != nil {
		t.Fatalf("put: %v", err)
	}
	msg, err := watch.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if structString(msg, "key") != "watched" {
		t.Fatalf("unexpected watch message: %v", msg)
	}
}

func TestChatBroadcast(t *testing.T) {
	client, authCtx, cleanup := newTestClient(t)
	defer cleanup()

	stream, err := client.Chat(authCtx)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if err := stream.Send(MustStruct(map[string]any{"room": "main", "user": "alice", "message": "hello"})); err != nil {
		t.Fatalf("send: %v", err)
	}
	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if structString(msg, "message") != "hello" {
		t.Fatalf("unexpected message: %v", msg)
	}
}

func TestAuthRejected(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := NewServer("test-token")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Serve(ctx, listener) }()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	defer conn.Close()
	client := NewClient(conn)

	_, err = client.Get(context.Background(), MustStruct(map[string]any{"key": "missing"}))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}

func TestChatClose(t *testing.T) {
	client, authCtx, cleanup := newTestClient(t)
	defer cleanup()
	stream, err := client.Chat(authCtx)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if err := stream.Send(MustStruct(map[string]any{"room": "main", "user": "alice", "message": "bye"})); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := stream.Recv(); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("recv: %v", err)
	}
}
