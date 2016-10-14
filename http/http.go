// Copyright 2015 The Chihaya Authors. All rights reserved.
// Use of this source code is governed by the BSD 2-Clause license,
// which can be found in the LICENSE file.

// Package http implements a BitTorrent tracker over the HTTP protocol as per
// BEP 3.
package http

import (
	"net"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/julienschmidt/httprouter"
	i2p "github.com/majestrate/chihaya/sam3"
	"github.com/tylerb/graceful"

	"github.com/majestrate/chihaya/config"
	"github.com/majestrate/chihaya/stats"
	"github.com/majestrate/chihaya/tracker"
)

// ResponseHandler is an HTTP handler that returns a status code.
type ResponseHandler func(http.ResponseWriter, *http.Request, httprouter.Params) (int, error)

// Server represents an HTTP serving torrent tracker.
type Server struct {

	// i2p related members
	sam         *i2p.SAM
	samKeys     *i2p.I2PKeys
	samSession  *i2p.StreamSession
	samListener *i2p.StreamListener

	config   *config.Config
	tracker  *tracker.Tracker
	grace    *graceful.Server
	stopping bool
}

// makeHandler wraps our ResponseHandlers while timing requests, collecting,
// stats, logging, and handling errors.
func makeHandler(handler ResponseHandler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		start := time.Now()
		httpCode, err := handler(w, r, p)
		duration := time.Since(start)

		var msg string
		if err != nil {
			msg = err.Error()
		} else if httpCode != http.StatusOK {
			msg = http.StatusText(httpCode)
		}

		if len(msg) > 0 {
			http.Error(w, msg, httpCode)
			stats.RecordEvent(stats.ErroredRequest)
		}

		if len(msg) > 0 || glog.V(2) {
			reqString := r.URL.Path + " " + r.RemoteAddr
			if glog.V(3) {
				reqString = r.URL.RequestURI() + " " + r.RemoteAddr
			}

			if len(msg) > 0 {
				glog.Errorf("[HTTP - %9s] %s (%d - %s)", duration, reqString, httpCode, msg)
			} else {
				glog.Infof("[HTTP - %9s] %s (%d)", duration, reqString, httpCode)
			}
		}

		stats.RecordEvent(stats.HandledRequest)
		stats.RecordTiming(stats.ResponseTime, duration)
	}
}

func (s *Server) I2PAddr() i2p.I2PAddr {
	return s.samKeys.Addr()
}

// newRouter returns a router with all the routes.
func newRouter(s *Server) *httprouter.Router {
	r := httprouter.New()

	if s.config.PrivateEnabled {
		r.GET("/users/:passkey/announce", makeHandler(s.serveAnnounce))
		r.GET("/users/:passkey/scrape", makeHandler(s.serveScrape))
	} else {
		r.GET("/announce", makeHandler(s.serveAnnounce))
		r.GET("/scrape", makeHandler(s.serveScrape))
	}
	r.GET("/", makeHandler(s.serveIndex))
	return r
}

// connState is used by graceful in order to gracefully shutdown. It also
// keeps track of connection stats.
func (s *Server) connState(conn net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		stats.RecordEvent(stats.AcceptedConnection)

	case http.StateClosed:
		stats.RecordEvent(stats.ClosedConnection)

	case http.StateHijacked:
		panic("connection impossibly hijacked")

	// Ignore the following cases.
	case http.StateActive, http.StateIdle:

	default:
		glog.Errorf("Connection transitioned to unknown state %s (%d)", state, state)
	}
}

func (s *Server) Setup() (err error) {
	addr := s.config.I2P.SAM.Addr
	glog.V(0).Info("Starting HTTP on i2p via ", addr)
	s.sam, err = i2p.NewSAM(addr)
	if err != nil {
		glog.Errorf("Failed to talk to I2P via %s: %s", addr, err)
		return
	}

	fname := s.config.I2P.SAM.Keyfile
	var keys i2p.I2PKeys
	glog.V(0).Info("Ensuring keyfile ", fname)
	keys, err = s.sam.EnsureKeyfile(fname)
	if err != nil {
		glog.Errorf("Could not persist/load keyfile %s: %s", fname, err)
		return
	}

	s.samKeys = &keys

	sess := s.config.I2P.SAM.Session
	opts := s.config.I2P.SAM.Opts
	glog.V(0).Info("Creating new Session with I2P")
	s.samSession, err = s.sam.NewStreamSession(sess, keys, opts.AsList())
	if err != nil {
		glog.Errorf("Could not create session with I2P: %s", err)
		return
	}

	glog.V(0).Info("Starting to Listen for I2P Connections")
	return
}

// Serve runs an HTTP server, blocking until the server has shut down.
func (s *Server) Serve() {
	glog.Infof("Serving on %s", s.I2PAddr().Base32())
	router := newRouter(s)
	serv := &http.Server{
		Handler:      router,
		ReadTimeout:  s.config.HTTPConfig.ReadTimeout.Duration,
		WriteTimeout: s.config.HTTPConfig.WriteTimeout.Duration,
	}
	l, err := s.samSession.Listen()
	if err == nil {
		// disable keepalive
		serv.SetKeepAlivesEnabled(false)
		err = serv.Serve(l)
	}
	glog.Error(err)
	glog.Info("HTTP server shut down cleanly")
}

// Stop cleanly shuts down the server.
func (s *Server) Stop() {
	if !s.stopping {
		s.grace.Stop(s.grace.Timeout)
	}
}

// NewServer returns a new HTTP server for a given configuration and tracker.
func NewServer(cfg *config.Config, tkr *tracker.Tracker) *Server {
	return &Server{
		config:  cfg,
		tracker: tkr,
	}
}
