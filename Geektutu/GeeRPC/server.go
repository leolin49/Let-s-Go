package geerpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/codec"
)

const MagicNumber = 0x3bef5c

// | Option{MagicNumber: xxx, CodecType: xxx}  | Header{ServiceMethod ...} | Body interface{} |
// | <------        JSON Encode       ------>  | <-------          CodeType           ------->|
// Message format:
// | Option | Header1 | Body1 | Header2 | Body2 | ...

type Option struct {
	MagicNumber 	int				// MagicNumber marks this's a geerpc request.
	CodecType   	codec.Type  	// client may choose different Codec to encode body.
	ConnectTimeout 	time.Duration	// 0 means no limit.
	HandleTimeout 	time.Duration	
}

var DefaultOption = &Option{
	MagicNumber: 	MagicNumber,
	CodecType: 	 	codec.GobType,
	ConnectTimeout: time.Second * 10,
}

// Server represents an RPC server.
type Server struct{
	serviceMap sync.Map
}

// NewServer returns a new RPC server.
func NewServer() *Server {
	return &Server{}
}

// DefaultServer is the default instance of *Server.
var DefaultServer = NewServer()

// Accept accepts connections on the listener and serves requests
// for each incoming connection.
func (s *Server) Accept(listener net.Listener) {
	// Waiting for socket connection from clients.
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		// One client binding one conn.
		go s.ServeConn(conn)
	}
}

// Accept accepts connections on the listener and serves requests
// for each incoming connection.
func Accept(listener net.Listener) { DefaultServer.Accept(listener) }

// ServeConn runs the server on a single connection.
// ServeConn blocks, serving the connection until the client hang up.
func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()
	// Read the option from client.
	var opt Option
	// NOTE: Decode(&opt) read from the conn cache buffer,
	// 		 if buffer is empty, block and wait for client.
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: option error:", err)
		return
	}
	// Check the option.
	if opt.MagicNumber != MagicNumber {
		// The conn is not geerpc conn.
		log.Printf("rpc server: invalid magic number %x", opt.MagicNumber)
		return
	}
	// f is a NewCodecFunc function.
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		// The server not implement the opt.CodecType.
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}

	// Use f for this client conn.
	s.serveCodec(f(conn), &opt)
}

// invalidRequest is a placeholder for response argv when error occurs.
var invalidRequest = struct{}{}

// servecodec use goroutine to handle requests.
// NOTE: Processing requests is concurrent, 
// 		 but reply request messages must be sent one by one, 
// 		 concurrency can easily lead to multiple reply messages intertwined, 
// 		 the client can not parse. 
// 		 Here locks (sending) are used to ensure that.
func (s *Server) serveCodec(cc codec.Codec, opt *Option) {
	sending := new(sync.Mutex)  // make sure to send a complete response.
	wg := new(sync.WaitGroup)	// wait until all request are handled. 

	// There are many requests in one connection.
	for {
		req, err := s.readRequest(cc)
		if err != nil {
			if req == nil {
				break // it's not possible to recover, so close the connection.
			}
			// Response for a invalidRequest with error info.
			req.h.Error = err.Error()
			s.sendResponse(cc, req.h, invalidRequest, sending)
			// Recover, continue to handle next request.
			continue
		}
		wg.Add(1)
		go s.handleRequest(cc, req, sending, wg, opt.HandleTimeout)
	}
	wg.Wait()
	_ = cc.Close()
}

// request stores all infomation of a rpc call.
type request struct {
	h		*codec.Header	// header of request.
	argv	reflect.Value	// argv of request.
	replyv  reflect.Value	// replyv of request.
	mtype	*methodType
	svc		*service
}

func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

func (s *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{
		h: h,
	}
	req.svc, req.mtype, err = s.findService(h.ServiceMethod)
	if err != nil {
		return nil, err
	}
	req.argv = req.mtype.newArgs()
	req.replyv = req.mtype.newReplyv()

	// make sure that argvi is a pointer,
	// ReadBody need a pointer as parameter.
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read body err:", err)
		return req, err
	}
	return req, nil
}

func (s *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	called := make(chan struct{})
	sent := make(chan struct{})
	go func() {
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.h.Error = err.Error()
			s.sendResponse(cc, req.h, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		s.sendResponse(cc, req.h, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()
	if timeout == 0 {	// no timeout limit.
		<- called
		<- sent
		return
	}
	select {
	case <- time.After(timeout):
		req.h.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
		s.sendResponse(cc, req.h, invalidRequest, sending)
	case <- called:
		<- sent
	}
}

func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (s *Server) Register(rcvr interface{}) error {
	service := newService(rcvr)
	if _, dup := s.serviceMap.LoadOrStore(service.name, service); dup {
		return errors.New("rpc server: service already defined: " + service.name)
	}
	return nil
}

// Register publishes the receiver's methods in the DefaultServer.
func Register(rcvr interface{}) error {
	return DefaultServer.Register(rcvr)
}

func (s *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := s.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service: " + serviceMethod)
		return
	}
	svc = svci.(*service)
	mtype = svc.method[methodName]
	if mtype == nil {
		err = errors.New("rpc server: can't find method: " + serviceMethod)
	}
	return
}

