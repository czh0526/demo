package rpc

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	set "gopkg.in/fatih/set.v0"
)

type CodecOption int

const (
	OptionMethodInvocation CodecOption = 1 << iota
	OptionSubscriptions                = 1 << iota
)

func NewServer() *Server {
	server := &Server{
		services: make(serviceRegistry),
		codecs:   set.New(set.ThreadSafe),
		run:      1,
	}
	return server
}

func (s *Server) RegisterName(name string, rcvr interface{}) error {
	if s.services == nil {
		s.services = make(serviceRegistry)
	}

	svc := new(service)
	svc.typ = reflect.TypeOf(rcvr)
	rcvrVal := reflect.ValueOf(rcvr)

	if name == "" {
		return fmt.Errorf("no service name for type %s", svc.typ.String())
	}
	if !isExported(reflect.Indirect(rcvrVal).Type().Name()) {
		return fmt.Errorf("%s is not exported", reflect.Indirect(rcvrVal).Type().Name())
	}

	// 提取服务的两类方法
	methods, subscriptions := suitableCallbacks(rcvrVal, svc.typ)

	// 服务之前被注册过，合并callbacks/subscription 集合
	if regsvc, present := s.services[name]; present {
		if len(methods) == 0 && len(subscriptions) == 0 {
			return fmt.Errorf("Service %T doesn't have any suitable methods/subscriptions to expose", rcvr)
		}
		for _, m := range methods {
			regsvc.callbacks[formatName(m.method.Name)] = m
		}
		for _, s := range subscriptions {
			regsvc.subscriptions[formatName(s.method.Name)] = s
		}
		// 处理结束
		return nil
	}

	// 服务第一次注册
	svc.name = name
	svc.callbacks, svc.subscriptions = methods, subscriptions

	if len(svc.callbacks) == 0 && len(svc.subscriptions) == 0 {
		return fmt.Errorf("Service %T doesn't have any suitable method/subscriptions to expose", rcvr)
	}

	// 注册
	s.services[svc.name] = svc
	return nil
}

func (s *Server) ServeCodec(codec ServerCodec, options CodecOption) {
	defer codec.Close()
	s.serveRequest(context.Background(), codec, false, options)
}

func (s *Server) ServeSingleRequest(ctx context.Context, codec ServerCodec, options CodecOption) {
	s.serveRequest(ctx, codec, true, options)
}

/*
 	完成一次数据通信请求，有四种模式

	 					|	Batch		|	Non Batch
	--------------------|---------------|--------------------
		 SingleShot		|				|
	--------------------|---------------|--------------------
		Non	SingleShot	|				|
	--------------------|---------------|--------------------
*/
func (s *Server) serveRequest(ctx context.Context, codec ServerCodec, singleShot bool, options CodecOption) error {
	// 在 Non SingleShot 模式中，用于控制 go routine 的终止
	var pend sync.WaitGroup

	defer func() {
		if err := recover(); err != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			log.Fatal(string(buf))
		}
		s.codecsMu.Lock()
		s.codecs.Remove(codec)
		s.codecsMu.Unlock()
	}()

	// 构建 Context, 并为订阅模式的服务调用放置 Notifier 对象
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if options&OptionSubscriptions == OptionSubscriptions {
		ctx = context.WithValue(ctx, notifierKey{}, newNotifier(codec))
	}

	// 获取资源锁，操作 codecs
	s.codecsMu.Lock()
	if atomic.LoadInt32(&s.run) != 1 {
		s.codecsMu.Unlock()
		return &shutdownError{}
	}
	s.codecs.Add(codec)
	s.codecsMu.Unlock()

	for atomic.LoadInt32(&s.run) == 1 {
		// 读取数据
		reqs, batch, err := s.readRequest(codec)
		if err != nil {
			if err.Error() != "EOF" {
				log.Debug(fmt.Sprintf("read error %v\n", err))
				codec.Write(codec.CreateErrorResponse(nil, err))
			}
			// 请求方出错，终止本次数据交互
			pend.Wait()
			return nil
		}

		log.Debug("rpcRequest ==> serverRequest ")
		for i, req := range reqs {
			log.Debugf("\t %d) %s", i, req)
		}

		// 处理服务中断事件
		if atomic.LoadInt32(&s.run) != 1 {
			err = &shutdownError{}
			if batch {
				resps := make([]interface{}, len(reqs))
				for i, r := range reqs {
					resps[i] = codec.CreateErrorResponse(&r.id, err)
				}
				codec.Write(resps)
			} else {
				codec.Write(codec.CreateErrorResponse(&reqs[0].id, err))
			}
			return nil
		}

		// 处理 SingleShot 的场景
		if singleShot {
			if batch {
				s.execBatch(ctx, codec, reqs)
			} else {
				s.exec(ctx, codec, reqs[0])
			}
			return nil
		}

		// 处理多次请求的场景
		pend.Add(1)
		go func(reqs []*serverRequest, batch bool) {
			defer pend.Done()
			if batch {
				s.execBatch(ctx, codec, reqs)
			} else {
				s.exec(ctx, codec, reqs[0])
			}
		}(reqs, batch)
	}

	return nil

}

func (s *Server) exec(ctx context.Context, codec ServerCodec, req *serverRequest) {
	var response interface{}
	var callback func()
	// 执行调用
	if req.err != nil {
		response = codec.CreateErrorResponse(&req.id, req.err)
	} else {
		log.Debugf("ativate service call ==> %s", req.callb.method.Name)
		response, callback = s.handle(ctx, codec, req)
	}

	// 返回结果
	if err := codec.Write(response); err != nil {
		log.Debug("write codec error: %v", err)
		codec.Close()
	}
	log.Debug("write codec ==>")
	log.Debugf("\t %#v", response)

	// 触发回调
	if callback != nil {
		callback()
	}
}

func (s *Server) execBatch(ctx context.Context, codec ServerCodec, requests []*serverRequest) {
	responses := make([]interface{}, len(requests))
	var callbacks []func()

	// 执行调用
	for i, req := range requests {
		if req.err != nil {
			responses[i] = codec.CreateErrorResponse(&req.id, req.err)
		} else {
			var callback func()
			if responses[i], callback = s.handle(ctx, codec, req); callback != nil {
				callbacks = append(callbacks, callback)
			}
		}
	}

	// 返回结果
	if err := codec.Write(responses); err != nil {
		log.Debugf("write codec error: %s", err)
	}

	log.Debug("write codec ==>")
	for i, response := range responses {
		log.Debugf("\t %d) %s", i, response)
	}

	// 执行回调
	for _, c := range callbacks {
		c()
	}
}

func (s *Server) handle(ctx context.Context, codec ServerCodec, req *serverRequest) (interface{}, func()) {
	if req.err != nil {
		return codec.CreateErrorResponse(&req.id, req.err), nil
	}

	// 处理退订
	if req.isUnsubscribe {
		if len(req.args) >= 1 && req.args[0].Kind() == reflect.String {
			notifier, supported := NotifierFromContext(ctx)
			if !supported {
				return codec.CreateErrorResponse(&req.id, &callbackError{ErrNotificationsUnsupported.Error()}), nil
			}

			// 撤销订阅
			subid := ID(req.args[0].String())
			if err := notifier.unsubscribe(subid); err != nil {
				return codec.CreateErrorResponse(&req.id, &callbackError{err.Error()}), nil
			}

			return codec.CreateResponse(req.id, true), nil
		}
		return codec.CreateErrorResponse(&req.id, &invalidParamsError{"Expected subscription id as first argument"}), nil
	}

	// 处理订阅
	if req.callb.isSubscribe {
		// 创建订阅
		subid, err := s.createSubscription(ctx, codec, req)
		if err != nil {
			return codec.CreateErrorResponse(&req.id, &callbackError{err.Error()}), nil
		}
		// 回调函数 ==> 激活订阅
		activateSub := func() {
			notifier, _ := NotifierFromContext(ctx)
			notifier.activate(subid, req.svcname)
		}

		return codec.CreateResponse(req.id, subid), activateSub
	}

	// 服务调用
	if len(req.args) != len(req.callb.argTypes) {
		rpcErr := &invalidParamsError{
			fmt.Sprintf("%s%s%s expects %d parameters, got %d",
				req.svcname, serviceMethodSeparator, req.callb.method.Name,
				len(req.callb.argTypes), len(req.args)),
		}
		return codec.CreateErrorResponse(&req.id, rpcErr), nil
	}

	arguments := []reflect.Value{req.callb.rcvr}
	if req.callb.hasCtx {
		arguments = append(arguments, reflect.ValueOf(ctx))
	}
	if len(req.args) > 0 {
		arguments = append(arguments, req.args...)
	}

	reply := req.callb.method.Func.Call(arguments)
	if len(reply) == 0 {
		return codec.CreateResponse(req.id, nil), nil
	}

	// 函数调用执行出错
	if req.callb.errPos >= 0 {
		if !reply[req.callb.errPos].IsNil() {
			e := reply[req.callb.errPos].Interface().(error)
			res := codec.CreateErrorResponse(&req.id, &callbackError{e.Error()})
			return res, nil
		}
	}
	return codec.CreateResponse(req.id, reply[0].Interface()), nil
}

func (s *Server) readRequest(codec ServerCodec) ([]*serverRequest, bool, Error) {
	// 读取 rpcRequest
	reqs, batch, err := codec.ReadRequestHeaders()
	if err != nil {
		return nil, batch, err
	}
	log.Debug("read codec <==")
	for i, req := range reqs {
		log.Debugf("\t %d) %s", i, req)
	}

	// 转换 rpcRequest ==> serverRequest
	requests := make([]*serverRequest, len(reqs))
	for i, r := range reqs {
		var ok bool
		var svc *service

		// 错误的 rpcRequest 对象
		if r.err != nil {
			requests[i] = &serverRequest{id: r.id, err: r.err}
			continue
		}

		// 处理取消订阅
		if r.isPubSub && strings.HasSuffix(r.method, unsubscribeMethodSuffix) {
			requests[i] = &serverRequest{id: r.id, isUnsubscribe: true}
			argTypes := []reflect.Type{reflect.TypeOf("")}
			if args, err := codec.ParseRequestArguments(argTypes, r.params); err == nil {
				requests[i].args = args
			} else {
				requests[i].err = &invalidParamsError{err.Error()}
			}
			continue
		}

		// 检测被调用的服务是否存在
		if svc, ok = s.services[r.service]; !ok {
			requests[i] = &serverRequest{id: r.id, err: &methodNotFoundError{r.service, r.method}}
			continue
		}

		// 检测被订阅的方法是否存在
		if r.isPubSub {
			if callb, ok := svc.subscriptions[r.method]; ok {
				requests[i] = &serverRequest{id: r.id, svcname: svc.name, callb: callb}
				if r.params != nil && len(callb.argTypes) > 0 {
					argTypes := []reflect.Type{reflect.TypeOf("")}
					argTypes = append(argTypes, callb.argTypes...)
					if args, err := codec.ParseRequestArguments(argTypes, r.params); err == nil {
						requests[i].args = args[1:]
					} else {
						requests[i].err = &invalidParamsError{err.Error()}
					}
				}
			} else {
				requests[i] = &serverRequest{id: r.id, err: &methodNotFoundError{r.service, r.method}}
			}
			continue
		}

		// 检测被调用方法是否存在
		if callb, ok := svc.callbacks[r.method]; ok {
			requests[i] = &serverRequest{id: r.id, svcname: svc.name, callb: callb}
			if r.params != nil && len(callb.argTypes) > 0 {
				if args, err := codec.ParseRequestArguments(callb.argTypes, r.params); err == nil {
					requests[i].args = args
				} else {
					requests[i].err = &invalidParamsError{err.Error()}
				}
			}
			continue
		}
		requests[i] = &serverRequest{id: r.id, err: &methodNotFoundError{r.service, r.method}}
	}
	return requests, batch, nil
}

func (s *Server) createSubscription(ctx context.Context, c ServerCodec, req *serverRequest) (ID, error) {
	args := []reflect.Value{req.callb.rcvr, reflect.ValueOf(ctx)}
	args = append(args, req.args...)
	reply := req.callb.method.Func.Call(args)

	if !reply[1].IsNil() {
		return "", reply[1].Interface().(error)
	}

	return reply[0].Interface().(*Subscription).ID, nil
}
