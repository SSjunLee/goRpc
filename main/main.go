package main

import (
	"encoding/json"
	"fmt"
	"log"
	"myrpc"
	"myrpc/codec"
	"net"
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

	con, err := net.Dial("tcp", <-addr)
	if err != nil {
		panic("net error")
	}
	defer func() {
		con.Close()
	}()
	json.NewEncoder(con).Encode(myrpc.DefaultOption)
	cc := codec.NewGobCodec(con)
	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServeiceMethod: "Foo.Sum",
			Seq:            uint64(i + 1),
		}
		cc.Write(h, fmt.Sprintf("rpc req %d", i+1))
		cc.ReadHeader(h)
		var res string
		cc.ReadBody(&res)
		log.Println(res)
	}

}
