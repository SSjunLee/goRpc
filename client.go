package myrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"myrpc/codec"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Call struct {
	Seq           uint64
	serviceMethod string
	Args          interface{}
	Reply         interface{}
	Error         error
	Done          chan *Call
}

func (c *Call) done() {
	c.Done <- c
}

type Client struct {
	cc       codec.Codec
	seq      uint64
	pending  map[uint64]*Call //存储未处理完的请求
	opt      *Option
	closing  bool
	shutdown bool
	sending  sync.Mutex //为了保证请求的有序发送，即防止出现多个请求报文混淆
	mu       sync.Mutex
	header   codec.Header
}

func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, Errshutdown
	}
	call.Seq = client.seq
	client.pending[client.seq] = call
	client.seq++
	return call.Seq, nil
}
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}
func (client *Client) terminateCall(err error) {
	client.sending.Lock()
	client.mu.Lock()
	defer func() {
		client.mu.Unlock()
		client.sending.Unlock()
	}()
	client.shutdown = true
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}

type clientResult struct {
	client *Client
	err    error
}

type newClientFunc func(net.Conn, *Option) (*Client, error)

func dialTimeout(f newClientFunc, network, address string, opts ...*Option) (client *Client, err error) {
	opt, err := prepareOption(opts...)
	if err != nil {
		return nil, err
	}
	con, err := net.DialTimeout(network, address, opt.ConnectionTimeOut)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			con.Close()
		}
	}()
	ch := make(chan clientResult)
	go func() {
		client, err := f(con, opt)
		ch <- clientResult{client, err}
	}()
	if opt.ConnectionTimeOut == 0 {
		res := <-ch
		return res.client, res.err
	}
	select {
	case <-time.After(opt.ConnectionTimeOut):
		return nil, fmt.Errorf("rpc client: connect timeout: expect within %s", opt.ConnectionTimeOut)
	case res := <-ch:
		return res.client, res.err
	}

}

func Dial(network, address string, opts ...*Option) (client *Client, err error) {
	return dialTimeout(NewClient, network, address, opts...)
}

func prepareOption(opts ...*Option) (*Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodeType == "" {
		opt.CodeType = DefaultOption.CodeType
	}
	return opt, nil
}

func NewClient(con net.Conn, opt *Option) (*Client, error) {
	newfunc := codec.NewCodecFuncMap[opt.CodeType]
	if newfunc == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodeType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	if err := json.NewEncoder(con).Encode(opt); err != nil {
		log.Println("rpc client: options error: ", err)
		con.Close()
		return nil, err
	}
	client := &Client{
		cc:      newfunc(con),
		seq:     1,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.recieve()
	return client, nil
}

var Errshutdown = errors.New("connection is shutdown")

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return Errshutdown
	}
	c.cc.Close()
	c.closing = true
	return nil
}

func (c *Client) IsAvaliable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.shutdown && !c.closing
}

func (client *Client) recieve() {
	var err error
	for err == nil {
		var h codec.Header
		if err := client.cc.ReadHeader(&h); err != nil {
			break
		}
		call := client.removeCall(h.Seq)
		switch {
		case call == nil:
			//可能是请求没有发送完整，或者因为其他原因被取消，但是服务端仍旧处理了。
			err = client.cc.ReadBody(nil)
		case h.Error != "":
			{
				//call 存在，但服务端处理出错
				err = client.cc.ReadBody(nil)
				call.Error = fmt.Errorf(h.Error)
				call.done()
			}
		default:
			{
				err = client.cc.ReadBody(call.Reply)
				//call 存在，服务端处理正常
				if err != nil {
					call.Error = errors.New("reading body " + err.Error())
				}
				call.done()
			}
		}
	}
	client.terminateCall(err)
}

func (client *Client) send(call *Call) {
	client.sending.Lock()
	defer client.sending.Unlock()
	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}
	client.header.ServeiceMethod = call.serviceMethod
	client.header.Seq = seq
	client.header.Error = ""

	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call := client.removeCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}

	call := &Call{
		serviceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call)
	return call
}

func (client *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := client.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-ctx.Done():
		client.removeCall(call.Seq)
		return errors.New("rpc client: call failed: " + ctx.Err().Error())
	case <-call.Done:
		return call.Error
	}
}

func NewHTTPClient(con net.Conn, opt *Option) (*Client, error) {
	io.WriteString(con, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", defaultRPCPath))
	response, err := http.ReadResponse(bufio.NewReader(con), &http.Request{Method: "CONNECT"})
	if err == nil && response.Status == connected {
		return NewClient(con, opt)
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + response.Status)
	}
	return nil, err
}

func DialHTTP(network, address string, opts ...*Option) (*Client, error) {
	return dialTimeout(NewHTTPClient, network, address, opts...)
}

// XDial calls different functions to connect to a RPC server
// according the first parameter rpcAddr.
// rpcAddr is a general format (protocol@addr) to represent a rpc server
// eg, http@10.0.0.1:7001, tcp@10.0.0.1:9999, unix@/tmp/geerpc.sock
func XDial(rpcAddr string, opts ...*Option) (*Client, error) {
	parts := strings.Split(rpcAddr, "@")
	if len(parts) != 2 {

	}
	protocol, addr := parts[0], parts[1]
	switch protocol {
	case "http":
		return DialHTTP("tcp", addr, opts...)
	default:
		return Dial(protocol, addr, opts...)
	}

}
