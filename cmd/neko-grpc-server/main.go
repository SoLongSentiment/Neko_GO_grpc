package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	nekogrpc "neko_grpc"
)

func main() {
	addr := ":17501"
	if value := os.Getenv("NEKO_GRPC_ADDR"); value != "" {
		addr = value
	}
	token := os.Getenv("NEKO_GRPC_TOKEN")
	if token == "" {
		token = "dev-token"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("neko_grpc listening on %s", addr)
	if err := nekogrpc.NewServer(token).Serve(ctx, ln); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
