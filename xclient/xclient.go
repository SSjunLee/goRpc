package xclient

import (
	"context"
	. "myrpc"
	"reflect"
	"sync"
)

type XClient struct {
	d       Discovery
	mode    SelectMode
	opt     *Option
	mu      sync.Mutex
	clients map[string]*Client
}

func NewXClient(d Discovery, mode SelectMode, opt *Option) *XClient {
	r := &XClient{d: d, mode: mode, opt: opt, clients: make(map[string]*Client)}
	return r
}

func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, client := range xc.clients {
		client.Close()
		delete(xc.clients, key)
	}
	return nil
}

func (xc *XClient) dial(addr string) (*Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	client, ok := xc.clients[addr]
	if ok && !client.IsAvaliable() {
		client.Close()
		delete(xc.clients, addr)
		client = nil
	}
	if client == nil {
		var err error
		client, err = XDial(addr, xc.opt)
		if err != nil {
			return nil, err
		}
		xc.clients[addr] = client
	}
	return client, nil
}

func (xc *XClient) call(rpcaddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := xc.dial(rpcaddr)
	if err != nil {
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply)
}

func (xc *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := xc.d.Get(xc.mode) //通过负载均衡策略获取rpc的地址
	if err != nil {
		return err
	}
	return xc.call(rpcAddr, ctx, serviceMethod, args, reply)
}

func (xc *XClient) BoardCast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := xc.d.GetAll()
	if err != nil {
		return nil
	}
	//并发情况下需要使用互斥锁保证 error 和 reply 能被正确赋值。
	var wg sync.WaitGroup
	var e error
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(ctx)
	replyDone := reply == nil //被多个协程并发访问，需要mu来保证互斥
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var replyBk interface{}
			if reply != nil {
				replyBk = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			mu.Lock()
			err := xc.call(rpcAddr, ctx, serviceMethod, args, replyBk)
			if err != nil && e == nil {
				e = err
				cancel() //此时会触发ctx.Down信号，终止其他协程中call的调用
			}
			if err == nil && !replyDone {
				//将replyBk的值拷贝给reply，即将全局执行结果指向局部执行结果
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(replyBk).Elem())
				replyDone = true
			}
			mu.Unlock()

		}(rpcAddr)
	}
	wg.Wait()
	return e
}
