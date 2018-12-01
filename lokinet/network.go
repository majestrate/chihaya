package lokinet

import (
	"context"
	"errors"
	"net"
	"strings"
)

type Network struct {
	resolver net.Resolver
}

func NewLokiNetwork(addr string) *Network {
	return &Network{
		resolver: net.Resolver{
			Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
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
	addrs, err := n.resolver.LookupAddr(ctx, h)
	if err != nil {
		return nil, err
	}
	found := make([]string, len(addrs))
	for idx := range addrs {
		found[idx] = strings.TrimSuffix(addrs[idx], ".")
	}
	return found, nil
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

func (n *Network) PublicAddr(ctx context.Context, l net.Listener) (string, error) {
	addr := l.Addr().String()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	addrs, err := n.ReverseDNS(ctx, addr)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", errors.New("no reverse dns")
	}
	return net.JoinHostPort(addrs[0], port), nil
}
