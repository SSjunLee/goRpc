package main

import (
	"context"
	"log"
	"myrpc"
	"net"
	"net/http"
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

func startServer(addr chan string) {
	var foo Foo
	myrpc.Register(&foo)
	lfd, _ := net.Listen("tcp", ":9999")
	myrpc.HandleHttp()
	addr <- lfd.Addr().String()
	http.Serve(lfd, nil)
}

func call(addrch chan string) {
	client, _ := myrpc.DialHTTP("tcp", <-addrch)
	defer client.Close()
	time.Sleep(time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{i, i * i}
			var res int
			//ctx, _ := context.WithTimeout(context.Background(), time.Second)
			if err := client.Call(context.Background(), "Foo.Sum", args, &res); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Printf("%d+%d=%d", args.Num1, args.Num2, res)
		}(i)
	}
	wg.Wait()
}

func main() {

	log.SetFlags(0)
	addrch := make(chan string)
	go call(addrch)
	startServer(addrch)
}
