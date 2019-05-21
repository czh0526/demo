package main

import (
	"encoding/hex"
	"flag"
	"strings"

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
		"/ip4/127.0.0.1/tcp/9001/ipfs/16Uiu2HAmN53Eh8PzDARdYTo8qxzHpMcpkUZnHLjm1WWdB9bMZjqg",
	} {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			panic(err)
		}
		defaultBootstrapAddrs = append(defaultBootstrapAddrs, ma)
	}
}

type addrList []maddr.Multiaddr

func (al *addrList) Set(value string) error {
	addr, err := maddr.NewMultiaddr(value)
	if err != nil {
		return err
	}

	*al = append(*al, addr)
	return nil
}

func (al *addrList) String() string {
	strs := make([]string, len(*al))
	for i, addr := range *al {
		strs[i] = addr.String()
	}
	return strings.Join(strs, ",")
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

type Config struct {
	PrivKey        crypto.PrivKey
	BootstrapPeers addrList
	ListenAddrs    addrList
}

func ParseFlags() (Config, error) {
	var skString string
	cfg := Config{}
	flag.StringVar(&skString, "sk", "", "host's private key.")
	flag.Var(&cfg.BootstrapPeers, "bootstrap", "Adds a peer multiaddress to the bootstrap list")
	flag.Var(&cfg.ListenAddrs, "listen", "Adds a multiaddress to the listen list")

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
		return cfg, err
	}

	return cfg, nil
}
