package myrpc

import (
	"fmt"
	"reflect"
	"testing"
)

func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assert failed..."+msg, v...))
	}
}

type Foo int
type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) Swap(args Args, reply *Args) error {
	*reply = Args{args.Num2, args.Num1}
	return nil
}

/*
func TestNewService(t *testing.T) {
	var foo Foo
	s := NewService(&foo)
	_assert(len(s.method) == 1, "wrong service Method, expect 1, but got %d", len(s.method))
	mType := s.method["Sum"]
	_assert(mType != nil, "wrong Method, Sum shouldn't nil")
}*/

func TestServiceCall(t *testing.T) {
	var foo Foo
	s := NewService(&foo)
	mType := s.method["Sum"]
	argv := mType.newArgV()
	replyV := mType.newReplyV()
	//log.Println(argv, replyV)
	argv.Set(reflect.ValueOf(Args{Num1: 1, Num2: 3}))
	err := s.call(mType, argv, replyV)
	_assert(err == nil && *replyV.Interface().(*int) == 4 && mType.numCalls == 1, "call fail")
}

func TestCall2(t *testing.T) {
	var foo Foo
	s := NewService(&foo)
	mType := s.method["Swap"]
	argv := mType.newArgV()
	replyV := mType.newReplyV()
	//log.Println(argv, replyV)
	argv.Set(reflect.ValueOf(Args{Num1: 1, Num2: 3}))
	err := s.call(mType, argv, replyV)
	_assert(err == nil, "")
	res := replyV.Interface().(*Args)
	fmt.Println(*res)

}
