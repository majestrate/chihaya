package sam3

import (
	"context"
	"errors"
	"net"

	"github.com/golang/glog"

	"github.com/majestrate/chihaya/config"
)

// implements network.Network
type Network struct {
	// i2p related members
	sam     *SAM
	keys    *I2PKeys
	session *StreamSession
	conf    config.I2PConfig
}

func (n *Network) Setup() (err error) {

	addr := n.conf.SAM.Addr
	glog.V(0).Info("Starting HTTP on i2p via ", addr)
	n.sam, err = NewSAM(addr)
	if err != nil {
		glog.Errorf("Failed to talk to I2P via %s: %s", addr, err)
		return
	}

	fname := n.conf.SAM.Keyfile
	var keys I2PKeys
	glog.V(0).Info("Ensuring keyfile ", fname)
	keys, err = n.sam.EnsureKeyfile(fname)
	if err != nil {
		glog.Errorf("Could not persist/load keyfile %s: %s", fname, err)
		return
	}

	n.keys = &keys

	sess := n.conf.SAM.Session
	opts := n.conf.SAM.Opts
	glog.V(0).Info("Creating new Session with I2P")
	n.session, err = n.sam.NewStreamSession(sess, keys, opts.AsList())
	if err != nil {
		glog.Errorf("Could not create session with I2P: %s", err)
		return
	}
	return
}

func NewI2PNetwork(conf config.I2PConfig) *Network {
	return &Network{
		conf: conf,
	}
}

func (n *Network) Listen(network, addr string) (l net.Listener, err error) {
	if network != "i2p" {
		return nil, errors.New("invalid network, is not i2p")
	}
	return n.session.Listen(n.conf.Listeners)
}

func (n *Network) GetPublicPrivateAddrs(reverse, forward string) (string, string) {
	return forward, reverse
}

func (n *Network) ReverseDNS(c context.Context, a string) ([]string, error) {
	addr := I2PAddr(a)
	return []string{addr.Base32()}, nil
}

func (n *Network) ForwardDNS(c context.Context, h string) ([]net.Addr, error) {
	addr, err := n.session.Lookup(h)
	if err != nil {
		return nil, err
	}
	return []net.Addr{addr}, nil
}

func (n *Network) PublicAddr(c context.Context, l net.Listener) (string, error) {
	addr := I2PAddr(l.Addr().String())
	return addr.Base32(), nil
}
