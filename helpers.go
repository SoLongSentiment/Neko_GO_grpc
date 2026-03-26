package nekogrpc

import "google.golang.org/protobuf/types/known/structpb"

func MustStruct(values map[string]any) *structpb.Struct {
	msg, err := structpb.NewStruct(values)
	if err != nil {
		panic(err)
	}
	return msg
}
