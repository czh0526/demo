package agent

import (
	"context"
	"fmt"
	"net"

	rpc "github.com/czh0526/demo/agent/rpc"
	discovery "github.com/libp2p/go-libp2p-discovery"
	host "github.com/libp2p/go-libp2p-host"
	kad_dht "github.com/libp2p/go-libp2p-kad-dht"
	inet "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	protocol "github.com/libp2p/go-libp2p-protocol"
)

type Agent struct {
	host             host.Host
	dht              *kad_dht.IpfsDHT
	routingDiscovery *discovery.RoutingDiscovery

	// websocket
	wsEndpoint string
	wsListener net.Listener
	wsHandler  *rpc.Server
}

func NewAgent(ctx context.Context,
	host host.Host,
	dht *kad_dht.IpfsDHT,
	cfg *Config) (*Agent, error) {

	// 构建 Discovery
	routingDiscovery := discovery.NewRoutingDiscovery(dht)

	agent := &Agent{
		host:             host,
		dht:              dht,
		routingDiscovery: routingDiscovery,
	}

	listener, handler, err := rpc.StartWSEndpoint(cfg.WsEndpoint, cfg.WsOrigins)
	if err != nil {
		return nil, err
	}
	fmt.Printf("wss service listening on: %q \n", cfg.WsEndpoint)

	agent.wsEndpoint = cfg.WsEndpoint
	agent.wsListener = listener
	agent.wsHandler = handler

	return agent, nil
}

func (agent *Agent) SetStreamHandler(protoId protocol.ID, handler inet.StreamHandler) {
	agent.host.SetStreamHandler(protoId, handler)
}

func (agent *Agent) Advertise(ctx context.Context, ns string) {
	// 循环广播，每6小时一次
	discovery.Advertise(ctx, agent.routingDiscovery, ns)
}

func (agent *Agent) FindPeers(ctx context.Context, ns string) (<-chan pstore.PeerInfo, error) {
	return agent.routingDiscovery.FindPeers(ctx, ns)
}

func (agent *Agent) FindPeer(ctx context.Context, pid peer.ID) (pstore.PeerInfo, error) {
	return agent.dht.FindPeer(ctx, pid)
}

func (agent *Agent) Connect(ctx context.Context, pi pstore.PeerInfo) error {
	return agent.host.Connect(ctx, pi)
}

func (agent *Agent) NewStream(ctx context.Context, pid peer.ID, protoIDs ...protocol.ID) (inet.Stream, error) {
	return agent.host.NewStream(ctx, pid, protoIDs...)
}

func (agent *Agent) RegisterSvc(name string, svc interface{}) error {
	return agent.wsHandler.RegisterName(name, svc)
}
