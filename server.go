package myrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"myrpc/codec"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber       int
	CodeType          codec.Type
	ConnectionTimeOut time.Duration
	HandleTimeOut     time.Duration
}

var DefaultOption = &Option{
	MagicNumber:       MagicNumber,
	CodeType:          codec.GobType,
	ConnectionTimeOut: time.Second * 10,
}

type Server struct {
	serviceMap sync.Map
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

func (this *Server) Accept(lis net.Listener) {
	for {
		con, err := lis.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		go this.ServerCon(con)
	}
}

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
}

func (this *Server) ServerCon(con net.Conn) {
	defer func() { con.Close() }()

	//获取配置项
	var option Option
	if err := json.NewDecoder(con).Decode(&option); err != nil {
		log.Println("rpc server: options error: ", err)
		return
	}
	log.Println("解码器为 ", option.CodeType)
	//比较魔数
	if option.MagicNumber != MagicNumber {
		log.Println("rpc server: magincNumber error: ")
		return
	}
	f := codec.NewCodecFuncMap[option.CodeType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", option.CodeType)
		return
	}
	cc := f(con)
	this.serverCodec(cc, &option)
}

var invalidRequest = struct{}{}

func (this *Server) serverCodec(cc codec.Codec, opt *Option) {
	wg := new(sync.WaitGroup)
	sending := new(sync.Mutex)
	for {
		req, err := this.readRequest(cc)
		if err != nil {
			if req == nil {
				break //it's not possible to recover, so close the connection
			} else {
				req.h.Error = err.Error()
				this.sendResponse(cc, req.h, invalidRequest, sending)
			}
		} else {
			go this.handleReq(req, wg, cc, sending, opt.HandleTimeOut)
			wg.Add(1)
		}
	}
	wg.Wait()
	cc.Close()
}

func (server *Server) handleReq(req *request, wg *sync.WaitGroup, cc codec.Codec, mtx *sync.Mutex, timeOut time.Duration) {
	defer wg.Done()
	called := make(chan struct{})
	sent := make(chan struct{})
	go func() {
		//log.Println("服务器处理请求 ", "消息header: ", req.h, "消息arg： ", req.argv.Elem())
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, mtx)
			return
		}
		server.sendResponse(cc, req.h, req.replyv.Interface(), mtx)
		sent <- struct{}{}
	}()
	if timeOut == 0 {
		called <- struct{}{}
		sent <- struct{}{}
	}

	select {
	case <-time.After(timeOut):
		req.h.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeOut)
		server.sendResponse(cc, req.h, invalidRequest, mtx)
	case <-called:
		<-sent
	}
}
func (*Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sendingMtx *sync.Mutex) {
	sendingMtx.Lock()
	defer sendingMtx.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

type request struct {
	h            *codec.Header
	argv, replyv reflect.Value
	mtype        *methodType
	svc          *service
}

func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := server.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	req.svc, req.mtype, err = server.findService(h.ServeiceMethod)

	req.argv = req.mtype.newArgV()
	req.replyv = req.mtype.newReplyV()
	// make sure that argvi is a pointer, ReadBody need a pointer as parameter
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read argv err:", err)
	}
	return req, nil
}

func (*Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

func (server *Server) Register(rcvr interface{}) error {
	s := NewService(rcvr)
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return errors.New("rpc: service already defined: " + s.name)
	}
	return nil
}

func (server *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot == -1 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service " + serviceName)
		return
	}
	svc = svci.(*service)
	mtype = svc.method[methodName]
	if mtype == nil {
		err = errors.New("rpc server: can't find method " + methodName)
	}
	return
}

func Register(rcvr interface{}) error {
	return DefaultServer.Register(rcvr)
}
