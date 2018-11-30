// Copyright 2015 The Chihaya Authors. All rights reserved.
// Use of this source code is governed by the BSD 2-Clause license,
// which can be found in the LICENSE file.

package http

import (
	"fmt"
	"io"
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/majestrate/chihaya/stats"
	"github.com/majestrate/chihaya/tracker/models"
)

func handleTorrentError(err error, w *Writer) (int, error) {
	if err == nil {
		return http.StatusOK, nil
	} else if models.IsPublicError(err) {
		w.WriteError(err)
		stats.RecordEvent(stats.ClientError)
		return http.StatusOK, nil
	}

	return http.StatusInternalServerError, err
}

func (s *Server) serveAnnounce(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	writer := &Writer{w}
	ann, err := s.newAnnounce(r, p)
	if err != nil {
		return handleTorrentError(err, writer)
	}

	return handleTorrentError(s.tracker.HandleAnnounce(ann, writer), writer)
}

func (s *Server) serveScrape(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	writer := &Writer{w}
	scrape, err := s.newScrape(r, p)
	if err != nil {
		return handleTorrentError(err, writer)
	}

	return handleTorrentError(s.tracker.HandleScrape(scrape, writer), writer)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	addr := s.ServerAddr()
	txt := fmt.Sprintf("bittorrent open tracker announce url http://%s/announce\n", addr)
	_, err := io.WriteString(w, txt)
	txt = fmt.Sprintf("to use:\n\nmktorrent -a http://%s/announce somedirectory\n", addr)
	_, err = io.WriteString(w, txt)
	return http.StatusOK, err
}
