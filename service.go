package myrpc

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

type methodType struct {
	method    reflect.Method
	ArgType   reflect.Type //第一个参数的类型
	ReplyType reflect.Type //第2个参数的类型
	NumCalls  uint64       //后续统计方法调用次数时会用到
}

func (m *methodType) newArgV() reflect.Value {
	var argv reflect.Value
	if m.ArgType.Kind() == reflect.Ptr {
		//m.ArgType.Elem() 表示指针所指的类型
		//根据指针所指的类型T创建出执行T的指针argv
		argv = reflect.New(m.ArgType.Elem())
	} else {
		//根据值类型T创建出值类型T元素argv
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}

//必须是指针类型
func (m *methodType) newReplyV() reflect.Value {
	replyV := reflect.New(m.ReplyType.Elem())
	//Kind 返回的是元类型：如struct，map，指针...
	//Elem 返回指针所指向的类型
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyV.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyV.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyV
}

type service struct {
	name   string
	typ    reflect.Type
	rcvr   reflect.Value //结构体的实例本身
	method map[string]*methodType
}

func NewService(rcvr interface{}) *service {

	s := new(service)
	s.rcvr = reflect.ValueOf(rcvr)
	s.name = reflect.Indirect(s.rcvr).Type().Name()
	s.typ = reflect.TypeOf(rcvr)

	//log.Println(s.rcvr)
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.name)
	}
	s.registerMethods()
	return s
}

func (s *service) registerMethods() {
	s.method = make(map[string]*methodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !IsExportedOrBuiltInType(argType) || !IsExportedOrBuiltInType(replyType) {
			continue
		}
		s.method[method.Name] = &methodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.name, method.Name)
	}
}

func IsExportedOrBuiltInType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func (s *service) call(m *methodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.NumCalls, 1)
	f := m.method.Func
	returnValues := f.Call([]reflect.Value{s.rcvr, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}
