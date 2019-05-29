package main

import (
	"encoding/hex"
	"flag"

	"github.com/czh0526/demo/agent"
	crypto "github.com/libp2p/go-libp2p-crypto"
	maddr "github.com/multiformats/go-multiaddr"
	multiaddr "github.com/multiformats/go-multiaddr"
)

var (
	defaultSk             string
	defaultBootstrapAddrs []maddr.Multiaddr
	defaultListenAddrs    []maddr.Multiaddr
)

func init() {
	defaultSk = "08021220b4fb22652891cb67650ee60969ca844ffca70088fcc391ce7d703fd1aa4268cc"

	for _, s := range []string{
		"/ip4/0.0.0.0/tcp/9002",
	} {
		ma, err := maddr.NewMultiaddr(s)
		if err != nil {
			panic(err)
		}
		defaultListenAddrs = append(defaultListenAddrs, ma)
	}

	for _, s := range []string{
		"/ip4/192.144.147.36/tcp/13002/ipfs/16Uiu2HAmTyymfLEXmTotxuhJtJLEZto9KyMrL9dseGamX7yR63n7",
		"/ip4/127.0.0.1/tcp/9001/ipfs/16Uiu2HAm1mUVoq2NTx7V62PZWpjF16tWQFiqZzRiainD7ANdZd9X",
	} {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			panic(err)
		}
		defaultBootstrapAddrs = append(defaultBootstrapAddrs, ma)
	}
}

func createPrivKey(hexString string) (crypto.PrivKey, error) {
	skBytes, err := hex.DecodeString(hexString)
	if err != nil {
		return nil, err
	}
	privKey, err := crypto.UnmarshalPrivateKey(skBytes)
	if err != nil {
		return nil, err
	}
	return privKey, nil
}

func ParseFlags() (*agent.Config, error) {
	var skString string
	cfg := agent.Config{}
	flag.StringVar(&skString, "sk", "", "host's private key.")
	flag.Var(&cfg.BootstrapPeers, "bootstrap", "Adds a peer multiaddress to the bootstrap list")
	flag.Var(&cfg.ListenAddrs, "listen", "Adds a multiaddress to the listen list")
	flag.StringVar(&cfg.WsEndpoint, "ws", ":8080", "web socket host:port")
	flag.Var(&cfg.WsOrigins, "origins", "web socket host:port")

	flag.Parse()
	if len(cfg.BootstrapPeers) == 0 {
		cfg.BootstrapPeers = append(cfg.BootstrapPeers, defaultBootstrapAddrs...)
	}
	if len(cfg.ListenAddrs) == 0 {
		cfg.ListenAddrs = append(cfg.ListenAddrs, defaultListenAddrs...)
	}
	if skString == "" {
		skString = defaultSk
	}

	var err error
	cfg.PrivKey, err = createPrivKey(skString)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
