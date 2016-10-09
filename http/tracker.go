// Copyright 2015 The Chihaya Authors. All rights reserved.
// Use of this source code is governed by the BSD 2-Clause license,
// which can be found in the LICENSE file.

package http

import (
	"net/http"
	"strconv"

	"github.com/golang/glog"

	i2p "github.com/majestrate/chihaya/sam3"

	"github.com/julienschmidt/httprouter"

	"github.com/majestrate/chihaya/http/query"
	"github.com/majestrate/chihaya/tracker/models"
)

// newAnnounce parses an HTTP request and generates a models.Announce.
func (s *Server) newAnnounce(r *http.Request, p httprouter.Params) (*models.Announce, error) {
	q, err := query.New(r.URL.RawQuery)
	if err != nil {
		return nil, err
	}

	event, _ := q.Params["event"]
	numWant := requestedPeerCount(q, s.config.NumWantFallback)

	infohash, exists := q.Params["info_hash"]
	if !exists {
		return nil, models.ErrMalformedRequest
	}

	peerID, exists := q.Params["peer_id"]
	if !exists {
		return nil, models.ErrMalformedRequest
	}

	port, err := q.Uint64("port")
	if err != nil {
		return nil, models.ErrMalformedRequest
	}

	left, err := q.Uint64("left")
	if err != nil {
		return nil, models.ErrMalformedRequest
	}

	dest, err := requestDest(q, r)
	if err != nil {
		return nil, models.ErrMalformedRequest
	}

	ep := models.Endpoint{dest.DestHash(), uint16(port)}

	downloaded, err := q.Uint64("downloaded")
	if err != nil {
		return nil, models.ErrMalformedRequest
	}

	uploaded, err := q.Uint64("uploaded")
	if err != nil {
		return nil, models.ErrMalformedRequest
	}

	return &models.Announce{
		Config:     s.config,
		Compact:    true,
		Downloaded: downloaded,
		Event:      event,
		Dest:       ep,
		Infohash:   infohash,
		Left:       left,
		NumWant:    numWant,
		Passkey:    p.ByName("passkey"),
		PeerID:     peerID,
		Uploaded:   uploaded,
	}, nil
}

// newScrape parses an HTTP request and generates a models.Scrape.
func (s *Server) newScrape(r *http.Request, p httprouter.Params) (*models.Scrape, error) {
	q, err := query.New(r.URL.RawQuery)
	if err != nil {
		return nil, err
	}

	if q.Infohashes == nil {
		if _, exists := q.Params["info_hash"]; !exists {
			// There aren't any infohashes.
			return nil, models.ErrMalformedRequest
		}
		q.Infohashes = []string{q.Params["info_hash"]}
	}

	return &models.Scrape{
		Config: s.config,

		Passkey:    p.ByName("passkey"),
		Infohashes: q.Infohashes,
	}, nil
}

// requestedPeerCount returns the wanted peer count or the provided fallback.
func requestedPeerCount(q *query.Query, fallback int) int {
	if numWantStr, exists := q.Params["numwant"]; exists {
		numWant, err := strconv.Atoi(numWantStr)
		if err != nil {
			return fallback
		}
		return numWant
	}

	return fallback
}

// obtain the "real" i2p destination from the remote request
func requestDest(q *query.Query, r *http.Request) (dest i2p.I2PAddr, err error) {
	addr := r.RemoteAddr
	dest, err = i2p.NewI2PAddrFromString(addr)

	if err != nil {
		glog.Errorf("bad destination in announce: %s", addr)
	}
	return
}
