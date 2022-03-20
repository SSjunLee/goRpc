package main

import (
	"fmt"
	"log"
	"myrpc"
	"net"
	"sync"
	"time"
)

func RunServer(addr chan string) {

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
			args := fmt.Sprintf("req %d", i+1)
			var res string
			if err := client.Call("Foo.Sum", args, &res); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Println("reply:", res)
		}(i)
	}
	wg.Wait()

}
