package nekogrpc

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

type BenchScenario struct {
	Name      string        `json:"name"`
	Ops       int64         `json:"ops"`
	Duration  time.Duration `json:"duration"`
	OpsPerSec float64       `json:"ops_per_sec"`
}

type BenchReport struct {
	Module    string          `json:"module"`
	StartedAt time.Time       `json:"started_at"`
	Duration  time.Duration   `json:"duration"`
	Results   []BenchScenario `json:"results"`
}

func RunBenchmarks(duration time.Duration) (BenchReport, error) {
	if duration <= 0 {
		duration = time.Second
	}
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
		return BenchReport{}, err
	}
	defer conn.Close()
	client := NewClient(conn)
	authCtx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer bench-token")

	report := BenchReport{
		Module:    "neko_grpc",
		StartedAt: time.Now().UTC(),
		Duration:  duration,
		Results:   make([]BenchScenario, 0, 2),
	}

	start := time.Now()
	var ops int64
	for time.Since(start) < duration {
		_, err := client.Put(authCtx, MustStruct(map[string]any{"key": "bench", "value": "hello"}))
		if err != nil {
			return BenchReport{}, err
		}
		ops++
	}
	elapsed := time.Since(start)
	report.Results = append(report.Results, BenchScenario{Name: "unary_put", Ops: ops, Duration: elapsed, OpsPerSec: float64(ops) / elapsed.Seconds()})

	stream, err := client.Chat(authCtx)
	if err != nil {
		return BenchReport{}, err
	}
	start = time.Now()
	ops = 0
	for time.Since(start) < duration {
		if err := stream.Send(MustStruct(map[string]any{"room": "bench", "user": "bench", "message": "ping"})); err != nil {
			return BenchReport{}, err
		}
		if _, err := stream.Recv(); err != nil {
			return BenchReport{}, err
		}
		ops++
	}
	elapsed = time.Since(start)
	report.Results = append(report.Results, BenchScenario{Name: "chat_roundtrip", Ops: ops, Duration: elapsed, OpsPerSec: float64(ops) / elapsed.Seconds()})
	return report, nil
}
