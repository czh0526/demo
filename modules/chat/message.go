package chat

import (
	"encoding/json"

	chat_pb "github.com/czh0526/demo/modules/chat/pb"
)

type Message struct {
	PeerId  string `json:"peerId"`
	Content string `json:"content"`
}

func (m Message) String() string {
	if b, err := json.Marshal(m); err == nil {
		return string(b)
	}
	return ""
}

func Map2Message(m map[string]interface{}) (*Message, bool) {
	var c1, p1 interface{}
	var c2, p2 string
	var ok bool
	if c1, ok = m["content"]; !ok {
		return nil, false
	}
	if c2, ok = c1.(string); !ok {
		return nil, false
	}
	if p1, ok = m["peerId"]; !ok {
		return nil, false
	}
	if p2, ok = p1.(string); !ok {
		return nil, false
	}

	return &Message{
		PeerId:  p2,
		Content: c2,
	}, true
}

func newMessage(peerId string, msg *chat_pb.Msg) *Message {
	return &Message{
		PeerId:  peerId,
		Content: msg.Content,
	}
}
