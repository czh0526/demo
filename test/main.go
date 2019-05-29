package main

import (
	"fmt"
	"os"

	"github.com/czh0526/demo/test/console"
	ipfslog "github.com/ipfs/go-log"
	logging "github.com/whyrusleeping/go-logging"
)

func init() {
	ipfslog.SetAllLoggers(logging.ERROR)
	ipfslog.SetLogLevel("rpc", "DEBUG")
	ipfslog.SetLogLevel("chat", "DEBUG")
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Error usage: ./apps <websocket url>")
		return
	}
	// 启动 Console
	consoleCfg := console.Config{
		URL:    os.Args[1],
		Proto:  "",
		Origin: "http://127.0.0.1/8082",
	}
	console, err := console.New(consoleCfg)
	if err != nil {
		panic(err)
	}

	// 进入等待消息状态
	if err := console.SubscribeIncomingMsgs(); err != nil {
		panic(err)
	}

	// 打印提示语，进入交互模式
	console.Welcome()
	go console.Interactive()

	select {}
}
