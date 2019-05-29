package agent

import (
	"errors"
	"strings"

	crypto "github.com/libp2p/go-libp2p-crypto"
	maddr "github.com/multiformats/go-multiaddr"
)

type wsOrigins []string

func (origins *wsOrigins) Set(value string) error {
	if len(value) > 0 {
		*origins = append(*origins, value)
		return nil
	}
	return errors.New("empty origin not allow.")
}

func (origins *wsOrigins) String() string {
	strs := make([]string, len(*origins))
	for i, origin := range *origins {
		strs[i] = origin
	}
	return strings.Join(strs, ",")
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

type Config struct {
	PrivKey        crypto.PrivKey
	BootstrapPeers addrList
	ListenAddrs    addrList
	WsEndpoint     string
	WsOrigins      wsOrigins
}
