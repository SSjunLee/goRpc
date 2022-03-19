package myrpc

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"myrpc/codec"
	"net"
	"reflect"
	"sync"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber int
	CodeType    codec.Type
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodeType:    codec.GobType,
}

type Server struct{}

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
	this.serverCodec(cc)
}

var invalidRequest = struct{}{}

func (this *Server) serverCodec(cc codec.Codec) {
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
			go this.handleReq(req, wg, cc, sending)
			wg.Add(1)
		}
	}
	wg.Wait()
	cc.Close()
}

func (this *Server) handleReq(req *request, wg *sync.WaitGroup, cc codec.Codec, mtx *sync.Mutex) {
	defer wg.Done()
	log.Println("处理请求 ", req.h, req.argv.Elem())
	req.replyV = reflect.ValueOf(fmt.Sprintf("response to %d", req.h.Seq))
	this.sendResponse(cc, req.h, req.replyV.Interface(), mtx)
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
	argv, replyV reflect.Value
}

func (this *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := this.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	req.argv = reflect.New(reflect.TypeOf(""))
	if err = cc.ReadBody(req.argv.Interface()); err != nil {
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
