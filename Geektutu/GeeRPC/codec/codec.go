package codec

import "io"

// Codec is an interface of encode and decode.
// Different encoding and decoding methods can be used 
// for the data to be transmitted. 
type Codec interface {
	io.Closer
	// Read and Eecode.
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	// Encode and Write.
	Write(*Header, interface{}) error
}

// Rpc header
type Header struct {
	ServiceMethod 	string // format "Service.Method"
	Seq				uint64 // sequence number chosen by client.
	Error			string // rpc server error info.
}

type NewCodecFunc func(io.ReadWriteCloser) Codec

// Different encoding and decoding methods.
// e.g. json, gob, protobuf ... 
type Type string

const (
	GobType  Type = "application/gob"
	JsonType Type = "application/json" // not implemented.
)

var NewCodecFuncMap map[Type]NewCodecFunc

///////////////////////
func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
	// TODO
	// NewCodecFuncMap[JsonType] = NewJsonCodec
}

