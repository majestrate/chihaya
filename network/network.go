package network

import (
	"context"
	"net"
)

type Network interface {
	// set up initial network connection
	Setup() error
	// make new listener
	Listen(network, addr string) (net.Listener, error)
	// get reverse dns for an address
	ReverseDNS(c context.Context, addr string) ([]string, error)
	// get forward dns for an address
	ForwardDNS(c context.Context, h string) ([]net.Addr, error)
	// get pub/priv addresses
	GetPublicPrivateAddrs(reverse, forward string) (string, string)
	// get public address for listener
	PublicAddr(c context.Context, l net.Listener) (string, error)
}
