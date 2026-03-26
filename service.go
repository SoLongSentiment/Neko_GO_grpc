package nekogrpc

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type Metrics struct {
	unaryCalls  atomic.Int64
	streamCalls atomic.Int64
	authRejects atomic.Int64
}

type Service struct {
	mu          sync.RWMutex
	documents   map[string]*structpb.Struct
	versions    map[string]int64
	watchers    map[string]map[uint64]chan *structpb.Struct
	chatRooms   map[string]map[uint64]chan *structpb.Struct
	nextWatcher atomic.Uint64
	metrics     *Metrics
}

func NewService() *Service {
	return &Service{
		documents: make(map[string]*structpb.Struct),
		versions:  make(map[string]int64),
		watchers:  make(map[string]map[uint64]chan *structpb.Struct),
		chatRooms: make(map[string]map[uint64]chan *structpb.Struct),
		metrics:   &Metrics{},
	}
}

type NekoServiceServer interface {
	Put(context.Context, *structpb.Struct) (*structpb.Struct, error)
	Get(context.Context, *structpb.Struct) (*structpb.Struct, error)
	Watch(*structpb.Struct, NekoService_WatchServer) error
	Chat(NekoService_ChatServer) error
}

type NekoService_WatchServer interface {
	Send(*structpb.Struct) error
	grpc.ServerStream
}

type NekoService_ChatServer interface {
	Send(*structpb.Struct) error
	Recv() (*structpb.Struct, error)
	grpc.ServerStream
}

type watchServer struct{ grpc.ServerStream }
type chatServer struct{ grpc.ServerStream }

func (x *watchServer) Send(msg *structpb.Struct) error { return x.ServerStream.SendMsg(msg) }
func (x *chatServer) Send(msg *structpb.Struct) error  { return x.ServerStream.SendMsg(msg) }
func (x *chatServer) Recv() (*structpb.Struct, error) {
	msg := &structpb.Struct{}
	if err := x.ServerStream.RecvMsg(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func RegisterNekoService(registrar grpc.ServiceRegistrar, srv NekoServiceServer) {
	registrar.RegisterService(&grpc.ServiceDesc{
		ServiceName: "neko.grpc.NekoService",
		HandlerType: (*NekoServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "Put", Handler: _NekoService_Put_Handler},
			{MethodName: "Get", Handler: _NekoService_Get_Handler},
		},
		Streams: []grpc.StreamDesc{
			{StreamName: "Watch", Handler: _NekoService_Watch_Handler, ServerStreams: true},
			{StreamName: "Chat", Handler: _NekoService_Chat_Handler, ServerStreams: true, ClientStreams: true},
		},
	}, srv)
}

func _NekoService_Put_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := &structpb.Struct{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(NekoServiceServer).Put(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/neko.grpc.NekoService/Put"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(NekoServiceServer).Put(ctx, req.(*structpb.Struct))
	}
	return interceptor(ctx, in, info, handler)
}

func _NekoService_Get_Handler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := &structpb.Struct{}
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(NekoServiceServer).Get(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/neko.grpc.NekoService/Get"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(NekoServiceServer).Get(ctx, req.(*structpb.Struct))
	}
	return interceptor(ctx, in, info, handler)
}

func _NekoService_Watch_Handler(srv any, stream grpc.ServerStream) error {
	msg := &structpb.Struct{}
	if err := stream.RecvMsg(msg); err != nil {
		return err
	}
	return srv.(NekoServiceServer).Watch(msg, &watchServer{ServerStream: stream})
}

func _NekoService_Chat_Handler(srv any, stream grpc.ServerStream) error {
	return srv.(NekoServiceServer).Chat(&chatServer{ServerStream: stream})
}

func (s *Service) Put(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	key := structString(req, "key")
	if key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	value := structValue(req, "value")

	doc, err := structpb.NewStruct(map[string]any{
		"key":     key,
		"value":   value,
		"version": 0,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	s.mu.Lock()
	s.versions[key]++
	version := s.versions[key]
	doc.Fields["version"] = structpb.NewNumberValue(float64(version))
	s.documents[key] = doc
	watchers := cloneSubscriberMap(s.watchers[key])
	s.mu.Unlock()

	for _, ch := range watchers {
		select {
		case ch <- doc:
		default:
		}
	}
	return doc, nil
}

func (s *Service) Get(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	key := structString(req, "key")
	if key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	s.mu.RLock()
	doc, ok := s.documents[key]
	s.mu.RUnlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "document not found")
	}
	return doc, nil
}

func (s *Service) Watch(req *structpb.Struct, stream NekoService_WatchServer) error {
	key := structString(req, "key")
	if key == "" {
		return status.Error(codes.InvalidArgument, "key is required")
	}
	id, ch, cancel := s.addWatcher(key)
	defer cancel()

	s.mu.RLock()
	current := s.documents[key]
	s.mu.RUnlock()
	if current != nil {
		if err := stream.Send(current); err != nil {
			return err
		}
	}

	for {
		select {
		case <-stream.Context().Done():
			_ = id
			return stream.Context().Err()
		case doc := <-ch:
			if err := stream.Send(doc); err != nil {
				return err
			}
		}
	}
}

func (s *Service) Chat(stream NekoService_ChatServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	room := structString(first, "room")
	if room == "" {
		return status.Error(codes.InvalidArgument, "room is required")
	}
	id, ch, cancel := s.addChatSubscriber(room)
	defer cancel()

	go func() {
		for msg := range ch {
			_ = stream.Send(msg)
		}
	}()

	if err := s.broadcastChat(room, first); err != nil {
		return err
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = id
				return nil
			}
			return err
		}
		if err := s.broadcastChat(room, msg); err != nil {
			return err
		}
	}
}

func (s *Service) addWatcher(key string) (uint64, chan *structpb.Struct, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextWatcher.Add(1)
	ch := make(chan *structpb.Struct, 8)
	if s.watchers[key] == nil {
		s.watchers[key] = make(map[uint64]chan *structpb.Struct)
	}
	s.watchers[key][id] = ch
	return id, ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.watchers[key], id)
		close(ch)
	}
}

func (s *Service) addChatSubscriber(room string) (uint64, chan *structpb.Struct, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextWatcher.Add(1)
	ch := make(chan *structpb.Struct, 16)
	if s.chatRooms[room] == nil {
		s.chatRooms[room] = make(map[uint64]chan *structpb.Struct)
	}
	s.chatRooms[room][id] = ch
	return id, ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.chatRooms[room], id)
		close(ch)
	}
}

func (s *Service) broadcastChat(room string, msg *structpb.Struct) error {
	s.mu.RLock()
	subscribers := cloneSubscriberMap(s.chatRooms[room])
	s.mu.RUnlock()
	for _, ch := range subscribers {
		select {
		case ch <- msg:
		default:
		}
	}
	return nil
}

func cloneSubscriberMap[T any](in map[uint64]chan T) map[uint64]chan T {
	out := make(map[uint64]chan T, len(in))
	for id, ch := range in {
		out[id] = ch
	}
	return out
}

func structString(msg *structpb.Struct, key string) string {
	if msg == nil || msg.Fields[key] == nil {
		return ""
	}
	return msg.Fields[key].GetStringValue()
}

func structValue(msg *structpb.Struct, key string) any {
	if msg == nil || msg.Fields[key] == nil {
		return nil
	}
	return msg.Fields[key].AsInterface()
}
