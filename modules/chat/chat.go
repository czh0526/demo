package chat

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/czh0526/demo/agent"
	"github.com/czh0526/demo/agent/rpc"
	chat_pb "github.com/czh0526/demo/modules/chat/pb"
	"github.com/gogo/protobuf/proto"
	ipfslog "github.com/ipfs/go-log"
	inet "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	protocol "github.com/libp2p/go-libp2p-protocol"
)

var log = ipfslog.Logger("chat")

var PROTO_CHAT = "/chat/1.0.0"

type Chat struct {
	groups          []string
	friends         map[peer.ID]inet.Stream
	msgIncomings    map[peer.ID]chan *Message
	incomingMsgChan chan *Message
	agent           *agent.Agent
}

func New(ctx context.Context,
	groups []string,
	agent *agent.Agent) *Chat {

	// 加入组
	for _, group := range groups {
		agent.Advertise(ctx, group)
	}

	chat := &Chat{
		groups:          groups,
		friends:         make(map[peer.ID]inet.Stream),
		agent:           agent,
		msgIncomings:    make(map[peer.ID]chan *Message),
		incomingMsgChan: make(chan *Message),
	}

	return chat
}

func (c *Chat) RegisterService(agent *agent.Agent) {
	agent.SetStreamHandler(protocol.ID(PROTO_CHAT), c.handleNewStream)
	agent.RegisterSvc("chat", c)
}

func (c *Chat) ConnectPeer(ctx context.Context, pid peer.ID) error {
	fmt.Printf("获取 <%s> 的 Address \n", pid.Pretty())
	pi, err := c.agent.FindPeer(ctx, pid)
	if err != nil {
		return err
	}

	fmt.Printf("根据 Address 建立连接 \n")
	if err := c.agent.Connect(ctx, pi); err != nil {
		return err
	}

	return nil
}

func (c *Chat) SendMessage(pid peer.ID, msg string) error {
	stream, err := c.agent.NewStream(context.Background(), pid, protocol.ID(PROTO_CHAT))
	if err != nil {
		return err
	}

	message := chat_pb.Msg{}
	message.Content = msg
	data, err := proto.Marshal(&message)
	if err != nil {
		log.Errorf("Error: %s \n", err)
		return err
	}
	_, err = stream.Write(data)
	if err != nil {
		log.Errorf("Error: %s \n", err)
		return err
	}
	log.Debugf("SendMessage '%s' ==> %s", msg, pid.Pretty())

	stream.Close()
	return nil
}

func (c *Chat) readMessage(stream inet.Stream) (*chat_pb.Msg, error) {
	for {
		var msg chat_pb.Msg
		data, err := ioutil.ReadAll(stream)
		if err != nil {
			log.Errorf("Error: %s ", err)
			return nil, err
		}
		if err := proto.Unmarshal(data, &msg); err != nil {
			log.Errorf("Error: %s ", err)
			return nil, err
		}

		return &msg, nil
	}
}

func (c *Chat) JoinGroup(ctx context.Context, groupName string) {
	// 查找 group 中的其它节点
	fmt.Printf("Searching <%s>'s other peers ... \n", groupName)
	_, err := c.agent.FindPeers(ctx, groupName)
	if err != nil {
		panic(err)
	}
}

func (c *Chat) handleNewStream(stream inet.Stream) {
	log.Debugf("Chat.handleChatStream() was called.")
	peerId := stream.Conn().RemotePeer()

	// 读取、分派消息
	pbMsg, err := c.readMessage(stream)
	if err != nil {
		log.Errorf("readMsg error: %s ", err)
		return
	}

	log.Debugf("read message <== '%s' from %s", pbMsg, stream.Conn().RemotePeer().Pretty())

	// 通知 NotifyIncomingMsg(), 处理新消息
	c.incomingMsgChan <- newMessage(peerId.Pretty(), pbMsg)
}

func (c *Chat) NotifyIncomingMessages(ctx context.Context) (*rpc.Subscription, error) {
	log.Debugf("HandleIncomingMessages() was called.")
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}

	subscription := notifier.CreateSubscription()
	go func() {
		for msg := range c.incomingMsgChan {
			log.Debugf("notify incoming message.")
			if err := notifier.Notify(subscription.ID, msg); err != nil {
				log.Error("NotifyIncomingMessages()'s goruntine exits.")
				return
			}
		}
	}()

	return subscription, nil
}
