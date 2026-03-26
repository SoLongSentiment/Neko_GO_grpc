package nekogrpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type NekoServiceClient interface {
	Put(context.Context, *structpb.Struct, ...grpc.CallOption) (*structpb.Struct, error)
	Get(context.Context, *structpb.Struct, ...grpc.CallOption) (*structpb.Struct, error)
	Watch(context.Context, *structpb.Struct, ...grpc.CallOption) (NekoService_WatchClient, error)
	Chat(context.Context, ...grpc.CallOption) (NekoService_ChatClient, error)
}

type NekoService_WatchClient interface {
	Recv() (*structpb.Struct, error)
	grpc.ClientStream
}

type NekoService_ChatClient interface {
	Send(*structpb.Struct) error
	Recv() (*structpb.Struct, error)
	grpc.ClientStream
}

type nekoServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewClient(cc grpc.ClientConnInterface) NekoServiceClient {
	return &nekoServiceClient{cc: cc}
}

func (c *nekoServiceClient) Put(ctx context.Context, in *structpb.Struct, opts ...grpc.CallOption) (*structpb.Struct, error) {
	out := &structpb.Struct{}
	err := c.cc.Invoke(ctx, "/neko.grpc.NekoService/Put", in, out, opts...)
	return out, err
}

func (c *nekoServiceClient) Get(ctx context.Context, in *structpb.Struct, opts ...grpc.CallOption) (*structpb.Struct, error) {
	out := &structpb.Struct{}
	err := c.cc.Invoke(ctx, "/neko.grpc.NekoService/Get", in, out, opts...)
	return out, err
}

func (c *nekoServiceClient) Watch(ctx context.Context, in *structpb.Struct, opts ...grpc.CallOption) (NekoService_WatchClient, error) {
	stream, err := c.cc.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/neko.grpc.NekoService/Watch", opts...)
	if err != nil {
		return nil, err
	}
	client := &watchClient{ClientStream: stream}
	if err := client.SendMsg(in); err != nil {
		return nil, err
	}
	if err := client.CloseSend(); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *nekoServiceClient) Chat(ctx context.Context, opts ...grpc.CallOption) (NekoService_ChatClient, error) {
	stream, err := c.cc.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/neko.grpc.NekoService/Chat", opts...)
	if err != nil {
		return nil, err
	}
	return &chatClient{ClientStream: stream}, nil
}

type watchClient struct{ grpc.ClientStream }
type chatClient struct{ grpc.ClientStream }

func (x *watchClient) Recv() (*structpb.Struct, error) {
	msg := &structpb.Struct{}
	if err := x.ClientStream.RecvMsg(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (x *chatClient) Send(msg *structpb.Struct) error { return x.ClientStream.SendMsg(msg) }
func (x *chatClient) Recv() (*structpb.Struct, error) {
	msg := &structpb.Struct{}
	if err := x.ClientStream.RecvMsg(msg); err != nil {
		return nil, err
	}
	return msg, nil
}
