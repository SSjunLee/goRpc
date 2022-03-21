package main

import (
	"context"
	"log"
	"myrpc"
	"net"
	"sync"
	"time"
)

type Foo int
type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func RunServer(addr chan string) {
	var foo Foo
	err := myrpc.Register(foo)
	if err != nil {
		log.Fatal("register error", err)
		return
	}

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network fatel", err)
		return
	}

	log.Println("server start... on ", l.Addr())
	addr <- l.Addr().String()
	myrpc.Accept(l)
}

func main() {

	log.SetFlags(0)
	addr := make(chan string)
	go RunServer(addr)
	client, _ := myrpc.Dial("tcp", <-addr)
	defer client.Close()
	time.Sleep(time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{i, i * i}
			var res int
			ctx, _ := context.WithTimeout(context.Background(), time.Second)
			if err := client.Call(ctx, "Foo.Sum", args, &res); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Printf("%d+%d=%d", args.Num1, args.Num2, res)
		}(i)
	}
	wg.Wait()

}
