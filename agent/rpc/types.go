package rpc

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	set "gopkg.in/fatih/set.v0"
)

type service struct {
	name          string
	typ           reflect.Type
	callbacks     callbacks
	subscriptions subscriptions
}

type serviceRegistry map[string]*service
type callbacks map[string]*callback
type subscriptions map[string]*callback

type Server struct {
	services serviceRegistry
	run      int32
	codecsMu sync.Mutex
	codecs   set.Interface
}

type ServerCodec interface {
	ReadRequestHeaders() ([]rpcRequest, bool, Error)
	ParseRequestArguments(argTypes []reflect.Type, params interface{}) ([]reflect.Value, Error)
	CreateResponse(id interface{}, reply interface{}) interface{}
	CreateErrorResponse(id interface{}, err Error) interface{}
	CreateNotification(id, namespace string, event interface{}) interface{}
	Write(msg interface{}) error
	Close()
	Closed() <-chan interface{}
}
type rpcRequest struct {
	service  string
	method   string
	id       interface{}
	isPubSub bool
	params   interface{}
	err      Error
}

func (r rpcRequest) String() string {
	return fmt.Sprintf("rpcRequest: {id: %v, service: %s, method: %s, isPubSub: %v, params: %v}", string(*r.id.(*json.RawMessage)), r.service, r.method, r.isPubSub, r.params)
}

type serverRequest struct {
	id            interface{}
	svcname       string
	callb         *callback
	args          []reflect.Value
	isUnsubscribe bool
	err           Error
}

func (s serverRequest) String() string {
	args := make([]string, len(s.args))
	for _, arg := range s.args {
		args = append(args, arg.String())
	}
	return fmt.Sprintf("serverRequest: {id: %v,  svcname: %s, callb: %s, args: %s, isUnsubscribe: %v}",
		string(*s.id.(*json.RawMessage)), s.svcname, s.callb.method.Name, args, s.isUnsubscribe)
}

type callback struct {
	rcvr        reflect.Value
	method      reflect.Method
	argTypes    []reflect.Type
	hasCtx      bool
	errPos      int
	isSubscribe bool
}

type Error interface {
	Error() string
	ErrorCode() int
}
