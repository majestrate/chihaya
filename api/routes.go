// Copyright 2015 The Chihaya Authors. All rights reserved.
// Use of this source code is governed by the BSD 2-Clause license,
// which can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"runtime"

	"github.com/julienschmidt/httprouter"

	"github.com/majestrate/chihaya/stats"
	"github.com/majestrate/chihaya/tracker/models"
)

const jsonContentType = "application/json; charset=UTF-8"

func handleError(err error) (int, error) {
	if err == nil {
		return http.StatusOK, nil
	} else if _, ok := err.(models.NotFoundError); ok {
		stats.RecordEvent(stats.ClientError)
		return http.StatusNotFound, nil
	} else if _, ok := err.(models.ClientError); ok {
		stats.RecordEvent(stats.ClientError)
		return http.StatusBadRequest, nil
	}
	return http.StatusInternalServerError, err
}

func (s *Server) check(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	// Attempt to ping the backend if private tracker is enabled.
	if s.config.PrivateEnabled {
		if err := s.tracker.Backend.Ping(); err != nil {
			return handleError(err)
		}
	}

	_, err := w.Write([]byte("STILL-ALIVE"))
	return handleError(err)
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	w.Header().Set("Content-Type", jsonContentType)

	var err error
	var val interface{}
	query := r.URL.Query()

	stats.DefaultStats.GoRoutines = runtime.NumGoroutine()

	if _, flatten := query["flatten"]; flatten {
		val = stats.DefaultStats.Flattened()
	} else {
		val = stats.DefaultStats
	}

	if _, pretty := query["pretty"]; pretty {
		var buf []byte
		buf, err = json.MarshalIndent(val, "", "  ")

		if err == nil {
			_, err = w.Write(buf)
		}
	} else {
		err = json.NewEncoder(w).Encode(val)
	}

	return handleError(err)
}

func (s *Server) getTorrent(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	infohash, err := url.QueryUnescape(p.ByName("infohash"))
	if err != nil {
		return http.StatusNotFound, err
	}

	torrent, err := s.tracker.FindTorrent(infohash)
	if err != nil {
		return handleError(err)
	}

	w.Header().Set("Content-Type", jsonContentType)
	e := json.NewEncoder(w)
	return handleError(e.Encode(torrent))
}

func (s *Server) putTorrent(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	var torrent models.Torrent
	err := json.NewDecoder(r.Body).Decode(&torrent)
	if err != nil {
		return http.StatusBadRequest, err
	}

	resp := make(map[string]interface{})
	err = s.tracker.PutTorrent(&torrent)
	resp["error"] = err

	if err == nil {
		// everything is gud
		// TODO: put more info into response?
	}

	w.Header().Set("Content-Type", jsonContentType)
	e := json.NewEncoder(w)
	return handleError(e.Encode(resp))
}

func (s *Server) delTorrent(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	infohash, err := url.QueryUnescape(p.ByName("infohash"))
	if err != nil {
		return http.StatusNotFound, err
	}

	resp := make(map[string]interface{})
	err = s.tracker.DeleteTorrent(infohash)
	resp["error"] = err

	if err == nil {
		// everything is gud
		// TODO: put more info into response?
	}

	w.Header().Set("Content-Type", jsonContentType)
	e := json.NewEncoder(w)
	return handleError(e.Encode(resp))
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	user, err := s.tracker.FindUser(p.ByName("passkey"))
	if err == models.ErrUserDNE {
		return http.StatusNotFound, err
	} else if err != nil {
		return http.StatusInternalServerError, err
	}

	w.Header().Set("Content-Type", jsonContentType)
	e := json.NewEncoder(w)
	return handleError(e.Encode(user))
}

func (s *Server) putUser(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	var user models.User
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		return http.StatusBadRequest, err
	}

	var madeUser *models.User
	resp := make(map[string]interface{})
	madeUser, err = s.tracker.RegisterUser(&user)
	resp["error"] = err

	if err == nil {
		// everything is gud
		resp["user"] = *madeUser
	}

	w.Header().Set("Content-Type", jsonContentType)
	e := json.NewEncoder(w)
	return handleError(e.Encode(resp))
}

func (s *Server) delUser(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	resp := make(map[string]interface{})
	err := s.tracker.DeleteUser(p.ByName("passkey"))
	resp["error"] = err

	if err == nil {
		// everything is gud
		// TODO: put more info into response?
	}

	w.Header().Set("Content-Type", jsonContentType)
	e := json.NewEncoder(w)
	return handleError(e.Encode(resp))
}

func (s *Server) getClient(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	if err := s.tracker.ClientApproved(p.ByName("clientID")); err != nil {
		return http.StatusNotFound, err
	}
	return http.StatusOK, nil
}

func (s *Server) putClient(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	s.tracker.Cache.PutClient(p.ByName("clientID"))
	return http.StatusOK, nil
}

func (s *Server) delClient(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	s.tracker.Cache.DeleteClient(p.ByName("clientID"))
	return http.StatusOK, nil
}

// list categories in json
func (s *Server) listCategories(w http.ResponseWriter, r *http.Request, p httprouter.Params) (int, error) {
	return http.StatusOK, nil
}
