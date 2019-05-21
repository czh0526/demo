package chat

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/czh0526/demo/agent"
	chat_pb "github.com/czh0526/demo/modules/chat/pb"
	"github.com/gogo/protobuf/proto"
	inet "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	protocol "github.com/libp2p/go-libp2p-protocol"
)

var PROTO_CHAT = "/chat/1.0.0"

type Chat struct {
	groups        []string
	friends       map[peer.ID]inet.Stream
	agent         *agent.Agent
	groupPeerChan <-chan pstore.PeerInfo
}

func New(ctx context.Context,
	groups []string,
	agent *agent.Agent) *Chat {

	// 加入组
	for _, group := range groups {
		agent.Advertise(ctx, group)
	}

	chat := &Chat{
		groups:  groups,
		friends: make(map[peer.ID]inet.Stream),
		agent:   agent,
	}

	agent.SetStreamHandler(protocol.ID(PROTO_CHAT), chat.handleChatStream)

	return chat
}

func (chat *Chat) ChatWithPeer(ctx context.Context, pid peer.ID) error {
	fmt.Printf("获取 <%s> 的 Address \n", pid.Pretty())
	pi, err := chat.agent.FindPeer(ctx, pid)
	if err != nil {
		return err
	}

	fmt.Printf("根据 Address 建立连接 \n")
	if err := chat.agent.Connect(ctx, pi); err != nil {
		return err
	}

	return nil
}

func (chat *Chat) SendMessage(pid peer.ID, msg string) error {
	stream, err := chat.agent.NewStream(context.Background(), pid, protocol.ID(PROTO_CHAT))
	if err != nil {
		return err
	}

	message := chat_pb.Msg{}
	message.Content = msg
	data, err := proto.Marshal(&message)
	if err != nil {
		fmt.Printf("Error: %s \n", err)
		return err
	}
	_, err = stream.Write(data)
	if err != nil {
		fmt.Printf("Error: %s \n", err)
		return err
	}

	stream.Close()
	return nil
}

func (chat *Chat) readMessage(stream inet.Stream) (string, error) {
	for {
		var msg chat_pb.Msg
		data, err := ioutil.ReadAll(stream)
		if err != nil {
			fmt.Printf("Error: %s \n", err)
			return "", err
		}
		if err := proto.Unmarshal(data, &msg); err != nil {
			fmt.Printf("Error: %s \n", err)
			return "", err
		}

		return msg.Content, nil
	}
}

func (chat *Chat) JoinGroup(ctx context.Context, groupName string) {
	// 查找 group 中的其它节点
	fmt.Printf("Searching <%s>'s other peers ... \n", groupName)
	_, err := chat.agent.FindPeers(ctx, groupName)
	if err != nil {
		panic(err)
	}
}

func (chat *Chat) handleChatStream(stream inet.Stream) {
	msg, err := chat.readMessage(stream)
	if err != nil {
		fmt.Printf("readMsg error: %s \n", err)
		return
	}

	fmt.Printf("\t %s <== %s \n", msg, stream.Conn().RemotePeer())
}
