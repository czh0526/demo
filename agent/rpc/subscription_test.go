package rpc

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"
)

type NotificationTestService struct {
	mu           sync.Mutex
	unsubscribed bool

	gotHangSubscriptionReq  chan struct{}
	unblockHangSubscription chan struct{}
}

func (s *NotificationTestService) Echo(i int) int {
	return i
}

func (s *NotificationTestService) wasUnsubCallbackCalled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unsubscribed
}

func (s *NotificationTestService) Unsubscribe(subid string) {
	s.mu.Lock()
	s.unsubscribed = true
	s.mu.Unlock()
}

func (s *NotificationTestService) SomeSubscription(ctx context.Context, n, val int) (*Subscription, error) {
	// 从 上下文中获取 notifier 对象
	notifier, supported := NotifierFromContext(ctx)
	if !supported {
		return nil, ErrNotificationsUnsupported
	}

	// 为请求创建一个 Subscription
	subscription := notifier.CreateSubscription()
	// 为请求启动一个 goroutine
	go func() {
		time.Sleep(5 * time.Second)
		// 将 [val ... val + i] 通知给 client
		for i := 0; i < n; i++ {
			if err := notifier.Notify(subscription.ID, val+i); err != nil {
				return
			}
		}

		select {
		case <-notifier.Closed():
			s.mu.Lock()
			s.unsubscribed = true
			s.mu.Unlock()
		case <-subscription.Err():
			s.mu.Lock()
			s.unsubscribed = true
			s.mu.Unlock()
		}
	}()

	return subscription, nil
}

func TestNotifications(t *testing.T) {
	// 构建 Server
	server := NewServer()
	// 构建 Service
	service := &NotificationTestService{}

	// 注册服务
	if err := server.RegisterName("eth", service); err != nil {
		t.Fatalf("unable to register test service %v", err)
	}

	// 构建通信链路
	clientConn, serverConn := net.Pipe()

	// 启动服务端
	go server.ServeCodec(NewJSONCodec(serverConn), OptionMethodInvocation|OptionSubscriptions)

	// 构建客户端的编/解码器
	out := json.NewEncoder(clientConn)
	in := json.NewDecoder(clientConn)

	n := 5
	val := 12345
	request := map[string]interface{}{
		"id":      1,
		"method":  "eth_subscribe",
		"version": "2.0",
		"params":  []interface{}{"someSubscription", n, val},
	}

	// Client 发送 Request
	if err := out.Encode(request); err != nil {
		t.Fatal(err)
	}

	var response jsonSuccessResponse
	if err := in.Decode(&response); err != nil {
		t.Fatal(err)
	}

	var ok bool
	if _, ok = response.Result.(string); !ok {
		t.Fatalf("expected subscription id, got %T", response.Result)
	}

	for i := 0; i < n; i++ {
		var notification jsonNotification
		if err := in.Decode(&notification); err != nil {
			t.Fatalf("%v", err)
		}

		if int(notification.Params.Result.(float64)) != val+i {
			t.Fatalf("expected %d, got %d", val+i, notification.Params.Result)
		}
	}

	clientConn.Close()
	time.Sleep(1 * time.Second)

	if !service.wasUnsubCallbackCalled() {
		t.Error("unsubscribe callback not called after closing connection")
	}
}
