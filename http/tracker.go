// Copyright 2015 The Chihaya Authors. All rights reserved.
// Use of this source code is governed by the BSD 2-Clause license,
// which can be found in the LICENSE file.

package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

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

	privAddr, pubAddr, err := s.getRealAddress(q, r)
	if err != nil {
		return nil, models.ErrMalformedRequest
	}

	downloaded, err := q.Uint64("downloaded")
	if err != nil {
		return nil, models.ErrMalformedRequest
	}

	uploaded, err := q.Uint64("uploaded")
	if err != nil {
		return nil, models.ErrMalformedRequest
	}

	a := &models.Announce{
		Config:     s.config,
		Compact:    true,
		Downloaded: downloaded,
		Event:      event,
		Infohash:   infohash,
		Left:       left,
		NumWant:    numWant,
		Passkey:    p.ByName("passkey"),
		PeerID:     peerID,
		Uploaded:   uploaded,
	}
	a.PrivAddr = privAddr
	a.PubAddr = pubAddr
	a.Port = uint16(port)
	return a, nil
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

// obtain the "real" address from a remote connection
func (s *Server) getRealAddress(q *query.Query, r *http.Request) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	addrs, err := s.network.ReverseDNS(ctx, r.RemoteAddr)
	if err != nil {
		return "", "", err
	}
	if len(addrs) == 0 {
		return "", "", errors.New("no reverse dns provided")
	}
	priv, pub := s.network.GetPublicPrivateAddrs(addrs[0], r.RemoteAddr)
	return priv, pub, nil
}
