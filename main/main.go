package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/czh0526/demo/agent"
	"github.com/czh0526/demo/modules/chat"
	ipfslog "github.com/ipfs/go-log"
	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	kad_dht "github.com/libp2p/go-libp2p-kad-dht"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	logging "github.com/whyrusleeping/go-logging"
)

func init() {
	ipfslog.SetAllLoggers(logging.ERROR)
	ipfslog.SetLogLevel("rpc", "DEBUG")
	ipfslog.SetLogLevel("chat", "DEBUG")
}

func main() {
	cfg, err := ParseFlags()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	host, dht, err := makeHostAndDHT(ctx, cfg)
	if err != nil {
		panic(err)
	}
	fmt.Printf("host listening on: %s@%s \n", host.ID(), host.Network().ListenAddresses())

	// 启动 Agent
	agent, err := agent.NewAgent(ctx, host, dht, cfg)
	if err != nil {
		panic(err)
	}
	fmt.Printf("agent <%s> started ... \n", host.ID())

	// 启动 Chat
	chat := chat.New(ctx, []string{"chat room 1"}, agent)
	chat.RegisterService(agent)

	select {}
}

func makeHostAndDHT(ctx context.Context, cfg *agent.Config) (host.Host, *kad_dht.IpfsDHT, error) {

	// 构建 Host
	host, err := libp2p.New(ctx, libp2p.Identity(cfg.PrivKey), libp2p.ListenAddrs(cfg.ListenAddrs...))
	if err != nil {
		return nil, nil, err
	}

	// 构建&启动 DHT
	dht, err := kad_dht.New(ctx, host)
	if err != nil {
		return nil, nil, err
	}
	if err := dht.Bootstrap(ctx); err != nil {
		return nil, nil, err
	}

	// 将 Host 接入网络
	var wg sync.WaitGroup
	for _, paddr := range cfg.BootstrapPeers {
		pi, err := pstore.InfoFromP2pAddr(paddr)
		if err != nil {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := host.Connect(ctx, *pi); err != nil {
				fmt.Printf("connect to %s error: %s \n", pi.String(), err)
			}
		}()
	}
	wg.Wait()

	return host, dht, nil
}
