package rpc

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrNotificationsUnsupported = errors.New("notifications not supported")
	ErrSubscriptionNotFound     = errors.New("subscription not found")
)

type ID string

type Subscription struct {
	ID        ID
	namespace string
	err       chan error
}

func (s *Subscription) Namespace() string {
	return s.namespace
}

func (s *Subscription) Err() <-chan error {
	return s.err
}

type notifierKey struct{}

type Notifier struct {
	codec    ServerCodec
	subMu    sync.RWMutex
	active   map[ID]*Subscription
	inactive map[ID]*Subscription
}

/*
	构建 Notifier对象, 主要包含
	1）编å/解码器
	2）subscription 队列
*/
func newNotifier(codec ServerCodec) *Notifier {
	return &Notifier{
		codec:    codec,
		active:   make(map[ID]*Subscription),
		inactive: make(map[ID]*Subscription),
	}
}

/*
	该方法由服务对象调用，来获取 Pub/Sub 相关的功能
*/
func NotifierFromContext(ctx context.Context) (*Notifier, bool) {
	n, ok := ctx.Value(notifierKey{}).(*Notifier)
	return n, ok
}

func (n *Notifier) CreateSubscription() *Subscription {
	s := &Subscription{ID: NewID(), err: make(chan error)}
	n.subMu.Lock()
	n.inactive[s.ID] = s
	n.subMu.Unlock()
	return s
}

func (n *Notifier) activate(id ID, namespace string) {
	n.subMu.Lock()
	defer n.subMu.Unlock()
	if sub, found := n.inactive[id]; found {
		sub.namespace = namespace
		n.active[id] = sub
		delete(n.inactive, id)
	}
}

func (n *Notifier) unsubscribe(id ID) error {
	n.subMu.Lock()
	defer n.subMu.Unlock()

	if s, found := n.active[id]; found {
		close(s.err)
		delete(n.active, id)
		return nil
	}
	return ErrSubscriptionNotFound
}

func (n *Notifier) Notify(id ID, data interface{}) error {
	n.subMu.RLock()
	defer n.subMu.RUnlock()

	sub, active := n.active[id]
	if active {
		notification := n.codec.CreateNotification(string(id), sub.namespace, data)
		fmt.Printf("write codec ==> %s\n", notification)
		if err := n.codec.Write(notification); err != nil {
			fmt.Printf("write codec error: %v \n", err)
			n.codec.Close()
			return err
		}
		fmt.Println("write codec ok")
	}
	return nil
}

func (n *Notifier) Closed() <-chan interface{} {
	return n.codec.Closed()
}
