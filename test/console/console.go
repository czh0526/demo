package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	chat "github.com/czh0526/demo/modules/chat"
	ipfslog "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-peer"
	colorable "github.com/mattn/go-colorable"
	"github.com/peterh/liner"
	"golang.org/x/net/websocket"
)

var (
	log = ipfslog.Logger("console")
)

var (
	onlyWhitespace = regexp.MustCompile(`^\s*$`)
	exit           = regexp.MustCompile(`^\s*exit\s*;*\s*$`)
)

const DefaultPrompt = "> "

type Config struct {
	Prompt   string
	Prompter UserPrompter
	Printer  io.Writer
	URL      string
	Origin   string
	Proto    string
}

type incomingMsgChannel struct {
	Conn    io.ReadWriteCloser
	Encoder *json.Encoder
	Decoder *json.Decoder
}

type Console struct {
	url             string
	origin          string
	proto           string
	prompt          string
	prompter        UserPrompter
	printer         io.Writer
	msgChannels     map[peer.ID]struct{}
	incomingMsgChan chan *incomingMsgChannel
}

func New(config Config) (*Console, error) {

	if config.Prompter == nil {
		config.Prompter = Stdin
	}
	if config.Prompt == "" {
		config.Prompt = DefaultPrompt
	}
	if config.Printer == nil {
		config.Printer = colorable.NewColorableStdout()
	}

	console := &Console{
		prompt:          config.Prompt,
		prompter:        config.Prompter,
		printer:         config.Printer,
		url:             config.URL,
		origin:          config.Origin,
		proto:           config.Proto,
		msgChannels:     make(map[peer.ID]struct{}),
		incomingMsgChan: make(chan *incomingMsgChannel),
	}

	// 启动 goroutine, 处理 Incoming Message Channel
	go console.handleIncomingMsgChannels()

	return console, nil
}

func (c *Console) connectAgent() (io.ReadWriteCloser, error) {
	conn, err := websocket.Dial(c.url, c.proto, c.origin)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (c *Console) Close() {
	close(c.incomingMsgChan)
}

func (c *Console) Welcome() {
	fmt.Fprintf(c.printer, "\n\nWelcome to the Demo Console!\n\n")
}

func (c *Console) Interactive() {
	var (
		prompt    = c.prompt
		indents   = 0
		input     = ""
		scheduler = make(chan string)
		abort     = make(chan struct{})
	)

	go func() {
		for {
			// 从 Channel 读取提示符，并等待用户在终端输入文字
			line, err := c.prompter.PromptInput(<-scheduler)
			if err != nil {
				if err == liner.ErrPromptAborted {
					// 处理 Ctrl-C
					prompt, indents, input = c.prompt, 0, ""
					scheduler <- ""
					abort <- struct{}{}
					continue
				}
				close(scheduler)
				return
			}
			// 将用户在终端输入的文字写入 Channel
			scheduler <- line
		}
	}()

	for {
		// 向 Channel 发送提示符，启动“获取用户在终端输入文字”的过程
		scheduler <- prompt
		select {
		case <-abort:
			fmt.Fprintln(c.printer, "caught interrupt, exiting")
			c.Close()
			return
		case line, ok := <-scheduler:
			// 处理用户输入的文字
			if !ok || (indents <= 0 && exit.MatchString(line)) {
				c.Close()
				return
			}
			if onlyWhitespace.MatchString(line) {
				continue
			}

			input += line + "\n"

			indents = countIndents(input)
			if indents <= 0 {
				prompt = c.prompt
			} else {
				prompt = strings.Repeat(".", indents*3) + " "
			}

			if indents <= 0 {
				if err := c.Evaluate(line); err != nil {
					c.Printf("error: %s \n", err)
				}
				input = ""
			}
		}
	}
}

func countIndents(input string) int {
	var (
		indents     = 0
		inString    = false
		strOpenChar = ' '   // keep track of the string open char to allow var str = "I'm ....";
		charEscaped = false // keep track if the previous char was the '\' char, allow var str = "abc\"def";
	)

	for _, c := range input {
		switch c {
		case '\\':
			// indicate next char as escaped when in string and previous char isn't escaping this backslash
			if !charEscaped && inString {
				charEscaped = true
			}
		case '\'', '"':
			if inString && !charEscaped && strOpenChar == c { // end string
				inString = false
			} else if !inString && !charEscaped { // begin string
				inString = true
				strOpenChar = c
			}
			charEscaped = false
		case '{', '(':
			if !inString { // ignore brackets when in string, allow var str = "a{"; without indenting
				indents++
			}
			charEscaped = false
		case '}', ')':
			if !inString {
				indents--
			}
			charEscaped = false
		default:
			charEscaped = false
		}
	}

	return indents
}

func (c *Console) Printf(format string, args ...interface{}) {
	fmt.Fprintf(c.printer, format, args...)
}

func (c *Console) Evaluate(statement string) error {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(c.printer, "[native] error: %v \n", r)
		}
	}()

	if strings.HasPrefix(statement, "send_msg") {
		var peerid string
		var msg string
		parts := strings.Split(strings.TrimSpace(statement), " ")
		if len(parts) != 3 {
			return errors.New("send_msg: <pid> <msg>")
		}
		peerid = parts[1]
		msg = parts[2]
		if err := c.sendMsg(peerid, msg); err != nil {
			return errors.New(fmt.Sprintf("sendMsg() error: %v", err))
		}

		return nil

	} else if statement == "ls" {
		idx := 1
		for pid, _ := range c.msgChannels {
			c.Printf("\t %d) %s \n", idx, pid.Pretty())
			idx++
		}

	} else if strings.HasPrefix(statement, "join_group:") {
		c.Printf("*) join_group: has not be implemented.\n")
	}
	return nil
}

func (c *Console) sendMsg(peerid string, msg string) error {
	var conn io.ReadWriteCloser
	var err error
	var pid peer.ID
	// 与 Agent 建立连接
	if conn, err = c.connectAgent(); err != nil {
		return err
	}
	defer conn.Close()
	out := json.NewEncoder(conn)
	in := json.NewDecoder(conn)

	pid, err = peer.IDB58Decode(peerid)
	if err != nil {
		return err
	}
	// 命令 Agent 向 peerid 发送消息
	request := map[string]interface{}{
		"version": "2.0",
		"id":      12345,
		"method":  "chat_sendMessage",
		"params":  []string{peerid, msg},
	}
	if err := out.Encode(request); err != nil {
		return err
	}

	// 处理 Agent 返回的结果
	var response map[string]interface{}
	if err := in.Decode(&response); err != nil {
		return err
	}

	var rspErr interface{}
	var ok bool
	if rspErr, ok = response["error"]; ok {
		return errors.New(fmt.Sprintf("remote error: %v", rspErr))
	}

	c.msgChannels[pid] = struct{}{}

	return nil
}

/*
	1) 构建 IncomingMsgChannel
	2）发送给 handleIncomingMsgChannel() 处理
*/
func (c *Console) SubscribeIncomingMsgs() error {
	var conn io.ReadWriteCloser
	var err error
	// 连接 Agent
	if conn, err = c.connectAgent(); err != nil {
		return err
	}
	out := json.NewEncoder(conn)
	in := json.NewDecoder(conn)

	// 命令 Agent 订阅 chat.HandleIncomingMessages() 发出的通知
	// 通知的格式是 chat.Message
	subscribe := map[string]interface{}{
		"version": "2.0",
		"id":      23456,
		"method":  "chat_subscribe",
		"params": []interface{}{
			"notifyIncomingMessages",
		},
	}

	if err := out.Encode(subscribe); err != nil {
		return err
	}

	// 处理订阅操作的结果
	var subid string
	response := jsonSuccessResponse{Result: subid}
	if err := in.Decode(&response); err != nil {
		return err
	}

	var ok bool
	if _, ok = response.Result.(string); !ok {
		return errors.New(fmt.Sprintf("expected subscription id, got %T", response.Result))
	}

	// 将后续的通知处理交给 handleIncomingMsgChannels()
	c.incomingMsgChan <- &incomingMsgChannel{
		Conn:    conn,
		Encoder: out,
		Decoder: in,
	}
	return nil
}

func (c *Console) handleIncomingMsgChannels() {
	var msg *chat.Message
	var m map[string]interface{}
	var ok bool
	var notification jsonNotification

	for {
		select {
		// 监听新建 MsgChannel 事件
		case msgChannel := <-c.incomingMsgChan:
			// 启动一个 goroutine，循环读取、处理 Message 消息
			go func() {
				for {
					if err := msgChannel.Decoder.Decode(&notification); err != nil {
						fmt.Println("[handleMessage]: read a bad notificaiton message.")
						return
					}

					if m, ok = notification.Params.Result.(map[string]interface{}); !ok {
						fmt.Printf("error type, notification.Params.Result is not map[string]interface{}\n")
						return
					}
					if msg, ok = chat.Map2Message(m); !ok {
						fmt.Printf("error type, expect chat.Message, but got %T \n", notification.Params.Result)
						return
					}

					pid, err := peer.IDB58Decode(msg.PeerId)
					if err != nil {
						fmt.Printf("pid is not correct, %s \n", msg.PeerId)
						return
					}
					fmt.Printf("%s <==  %s \n", msg.Content, msg.PeerId)
					c.msgChannels[pid] = struct{}{}
				}
			}()
		}
	}
}
