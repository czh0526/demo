package rpc

import (
	"net"

	ipfslog "github.com/ipfs/go-log"
)

var log = ipfslog.Logger("rpc")

func StartWSEndpoint(endpoint string, wsOrigins []string) (net.Listener, *Server, error) {
	rpcHandler := NewServer()

	var (
		listener net.Listener
		err      error
	)
	if listener, err = net.Listen("tcp", endpoint); err != nil {
		return nil, nil, err
	}
	// 指派路由规则、处理规则 + 绑定端口
	go NewWSServer(wsOrigins, rpcHandler).Serve(listener)

	return listener, rpcHandler, nil
}
