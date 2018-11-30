package lokinet

import (
	"context"
	"net"
)

type Network struct {
	resolver net.Resolver
}

func NewLokiNetwork(addr string) *Network {
	return &Network{
		resolver: net.Resolver{
			Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d *net.Dialer
				return d.DialContext(ctx, "udp", addr)
			},
		},
	}
}

func (n *Network) Setup() error {
	return nil
}

func (n *Network) Listen(network, addr string) (net.Listener, error) {
	return net.Listen(network, addr)
}

func (n *Network) ReverseDNS(ctx context.Context, a string) ([]string, error) {
	h, _, err := net.SplitHostPort(a)
	if err != nil {
		return nil, err
	}
	return n.resolver.LookupAddr(ctx, h)
}

func (n *Network) ForwardDNS(ctx context.Context, h string) (found []net.Addr, e error) {
	addrs, err := n.resolver.LookupIPAddr(ctx, h)
	if err != nil {
		e = err
		return
	}
	for idx := range addrs {
		found = append(found, &addrs[idx])
	}
	return
}

func (n *Network) GetPublicPrivateAddrs(reverse, forward string) (string, string) {
	h, _, _ := net.SplitHostPort(forward)
	return h, reverse
}
