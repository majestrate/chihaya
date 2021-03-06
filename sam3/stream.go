package sam3

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

// Represents a streaming session.
type StreamSession struct {
	samAddr   string              // address to the sam bridge (ipv4:port)
	id        string              // tunnel name
	conn      net.Conn            // connection to sam
	keys      I2PKeys             // i2p destination keys
	listeners []io.Closer         // active SteamListeners
	lookups   chan *lookupRequest // name lookup channel
}

// Returns the local tunnel name of the I2P tunnel used for the stream session
func (ss StreamSession) ID() string {
	return ss.id
}

func (ss *StreamSession) IsOpen() bool {
	return ss.conn != nil
}

func (ss *StreamSession) Close() error {
	for idx := range ss.listeners {
		ss.listeners[idx].Close()
	}
	ss.listeners = []io.Closer{}
	if ss.conn == nil {
		return nil
	}
	err := ss.conn.Close()
	ss.conn = nil
	return err
}

// Returns the I2P destination (the address) of the stream session
func (ss StreamSession) Addr() I2PAddr {
	return ss.keys.Addr()
}

// Returns the keys associated with the stream session
func (ss StreamSession) Keys() I2PKeys {
	return ss.keys
}

// Creates a new StreamSession with the I2CP- and streaminglib options as
// specified. See the I2P documentation for a full list of options.
func (sam *SAM) NewStreamSession(id string, keys I2PKeys, options []string) (*StreamSession, error) {
	conn, err := sam.newGenericSession("STREAM", id, keys, options, []string{})
	if err != nil {
		return nil, err
	}
	s := &StreamSession{sam.address, id, conn, keys, []io.Closer{}, make(chan *lookupRequest)}
	go s.runLookups()
	return s, nil
}

func (s *StreamSession) runLookups() {
	for s.IsOpen() {
		s.doNameLookup(<-s.lookups)
	}
}

// lookup name
func (s *StreamSession) Lookup(name string) (I2PAddr, error) {
	lookup := &lookupRequest{
		name: name,
		resp: make(chan lookupResult),
	}
	s.lookups <- lookup
	r := <-lookup.resp
	return r.addr, r.err
}

type lookupRequest struct {
	name string
	resp chan lookupResult
}

type lookupResult struct {
	addr I2PAddr
	err  error
}

func (ss *StreamSession) doNameLookup(req *lookupRequest) {
	if _, err := ss.conn.Write([]byte("NAMING LOOKUP NAME=" + req.name + "\n")); err != nil {
		ss.Close()
		req.resp <- lookupResult{I2PAddr(""), err}
		return
	}
	buf := make([]byte, 4096)
	n, err := ss.conn.Read(buf)
	if err != nil {
		ss.Close()
		req.resp <- lookupResult{I2PAddr(""), err}
		return
	}
	if n <= 13 || !strings.HasPrefix(string(buf[:n]), "NAMING REPLY ") {
		req.resp <- lookupResult{I2PAddr(""), errors.New("Failed to parse.")}
		return
	}
	s := bufio.NewScanner(bytes.NewReader(buf[13:n]))
	s.Split(bufio.ScanWords)

	errStr := ""
	for s.Scan() {
		text := s.Text()
		if text == "RESULT=OK" {
			continue
		} else if text == "RESULT=INVALID_KEY" {
			errStr += "Invalid key."
		} else if text == "RESULT=KEY_NOT_FOUND" {
			errStr += "Unable to resolve " + req.name
		} else if text == "NAME="+req.name {
			continue
		} else if strings.HasPrefix(text, "VALUE=") {
			req.resp <- lookupResult{I2PAddr(text[6:]), nil}
			return
		} else if strings.HasPrefix(text, "MESSAGE=") {
			errStr += " " + text[8:]
		} else {
			continue
		}
	}
	req.resp <- lookupResult{I2PAddr(""), errors.New(errStr)}
}

// create a new stream listener to accept inbound connections
func (s *StreamSession) Listen(n int) (*StreamListener, error) {
	l := &StreamListener{
		session:  s,
		id:       s.id,
		laddr:    s.keys.Addr(),
		accepted: make(chan acceptedConn, 128),
		run:      true,
	}
	s.listeners = append(s.listeners, l)
	if n <= 0 {
		n = 1
	}
	for n > 0 {
		go l.acceptLoop()
		n--
	}
	return l, nil
}

type acceptedConn struct {
	c   net.Conn
	err error
}

type StreamListener struct {
	// parent stream session
	session *StreamSession
	// our session id
	id string
	// our local address for this sam socket
	laddr I2PAddr
	// channel for accepted connection backlog
	accepted chan acceptedConn
	// run flag
	run bool
}

func (l *StreamListener) acceptLoop() {
	for l.run && l.session.IsOpen() {
		n, err := l.AcceptI2P()
		if l.accepted != nil {
			if err == nil {
				l.accepted <- acceptedConn{n, nil}
				continue
			}
		} else {
			return
		}
	}
}

// get our address
// implements net.Listener
func (l *StreamListener) Addr() net.Addr {
	return l.laddr
}

// implements net.Listener
func (l *StreamListener) Close() error {
	l.run = false
	chnl := l.accepted
	l.accepted = nil
	close(chnl)
	l.session = nil
	return nil
}

// implements net.Listener
func (l *StreamListener) Accept() (n net.Conn, err error) {
	a, ok := <-l.accepted
	if !ok {
		err = errors.New("i2p acceptor closed")
		return
	}
	n, err = a.c, a.err
	return
}

func (l *StreamListener) AcceptI2P() (*SAMConn, error) {
	if l.session == nil {
		return nil, errors.New("no i2p session for this listener")
	}
	s, err := NewSAM(l.session.samAddr)
	if err != nil {
		return nil, err
	}
	nc := s.conn
	fmt.Fprintf(nc, "STREAM ACCEPT ID=%s SILENT=false\n", l.id)
	var line string
	line, err = readLine(nc)
	if err != nil {
		nc.Close()
		return nil, err
	}
	scanner := bufio.NewScanner(strings.NewReader(line))
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		switch scanner.Text() {
		case "STREAM":
		case "STATUS":
			continue
		case "RESULT=OK":
			line, err = readLine(nc)
			if err != nil {
				nc.Close()
				return nil, err
			}
			nc.(*net.TCPConn).SetLinger(0)
			return &SAMConn{
				laddr: l.laddr,
				raddr: I2PAddr(line),
				conn:  nc,
			}, nil
		case "RESULT=CANT_REACH_PEER":
			nc.Close()
			return nil, errors.New("Can not reach peer")
		case "RESULT=I2P_ERROR":
			nc.Close()
			return nil, errors.New("I2P internal error")
		case "RESULT=INVALID_KEY":
			nc.Close()
			return nil, errors.New("Invalid key")
		case "RESULT=INVALID_ID":
			nc.Close()
			return nil, errors.New("Invalid tunnel ID")
		case "RESULT=TIMEOUT":
			nc.Close()
			return nil, errors.New("Timeout")
		default:
			nc.Close()
			return nil, errors.New("Unknown error: " + line)
		}
	}
	return nil, err
}
