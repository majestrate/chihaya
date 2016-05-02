// Copyright 2015 The Chihaya Authors. All rights reserved.
// Use of this source code is governed by the BSD 2-Clause license,
// which can be found in the LICENSE file.

// Package models implements the common data types used throughout a BitTorrent
// tracker.
package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/majestrate/chihaya/config"

	// for i2p announces
	i2p "github.com/majestrate/i2p-tools/sam3"
)

var (
	// ErrMalformedRequest is returned when a request does not contain the
	// required parameters needed to create a model.
	ErrMalformedRequest = ClientError("malformed request")

	// ErrBadRequest is returned when a request is invalid in the peer's
	// current state. For example, announcing a "completed" event while
	// not a leecher or a "stopped" event while not active.
	ErrBadRequest = ClientError("bad request")

	// ErrUserDNE is returned when a user does not exist.
	ErrUserDNE = NotFoundError("user does not exist")

	// ErrTorrentDNE is returned when a torrent does not exist.
	ErrTorrentDNE = NotFoundError("torrent does not exist")

	// ErrClientUnapproved is returned when a clientID is not in the whitelist.
	ErrClientUnapproved = ClientError("client is not approved")

	// ErrInvalidPasskey is returned when a passkey is not properly formatted.
	ErrInvalidPasskey = ClientError("passkey is invalid")
)

type ClientError string
type NotFoundError ClientError
type ProtocolError ClientError

func (e ClientError) Error() string   { return string(e) }
func (e NotFoundError) Error() string { return string(e) }
func (e ProtocolError) Error() string { return string(e) }

// IsPublicError determines whether an error should be propogated to the client.
func IsPublicError(err error) bool {
	_, cl := err.(ClientError)
	_, nf := err.(NotFoundError)
	_, pc := err.(ProtocolError)
	return cl || nf || pc
}

// PeerList represents a list of peers: either seeders or leechers.
type PeerList []Peer

// PeerKey is the key used to uniquely identify a peer in a swarm.
type PeerKey string

// NewPeerKey creates a properly formatted PeerKey given full i2p destination
func NewPeerKeyForDest(peerID string, addr i2p.I2PAddr) PeerKey {
	return NewPeerKey(peerID, addr.DestHash())
}

// NewPeerKey creates a properly formatted PeerKey given an i2p desthash
func NewPeerKey(peerID string, dhash i2p.I2PDestHash) PeerKey {
	return PeerKey(fmt.Sprintf("%s//%s", peerID, dhash))
}

// PeerID returns the PeerID section of a PeerKey.
func (pk PeerKey) PeerID() string {
	return strings.Split(string(pk), "//")[0]
}

// Dest returns the i2p destination hash of a PeerKey
func (pk PeerKey) Dest() (dhash i2p.I2PDestHash) {
	str := strings.Split(string(pk), "//")[1]
	dhash, _ = i2p.DestHashFromString(str)
	return
}

// Endpoint is an i2p destination hash with port optionally included
type Endpoint struct {
	Dest i2p.I2PDestHash `json:"-"`
	Port uint16          `json:"port"`
}

// Peer represents a participant in a BitTorrent swarm.
type Peer struct {
	ID           string `json:"id"`
	UserID       uint64 `json:"userId"`
	TorrentID    uint64 `json:"torrentId"`
	Uploaded     uint64 `json:"uploaded"`
	Downloaded   uint64 `json:"downloaded"`
	Left         uint64 `json:"left"`
	LastAnnounce int64  `json:"lastAnnounce"`
	Endpoint
}

// Key returns a PeerKey for the given peer.
func (p *Peer) Key() PeerKey {
	return NewPeerKey(p.ID, p.Dest)
}

// TorrentInfo holds all index metadata for a torrent on private trackers
type TorrentInfo struct {
	UserID      uint64   `json:"owner_user_id"`
	UploadDate  int64    `json:"uploaded"`
	Category    string   `json:"category"`
	TorrentName string   `json:"name"`
	Description string   `json:"desc"`
	Files       []string `json:"files"`
	Tags        []string `json:"tags"`
}

// Torrent represents a BitTorrent swarm and its metadata.
type Torrent struct {
	ID       uint64 `json:"id"`
	Infohash string `json:"infohash"`

	Seeders  *PeerMap `json:"seeders"`
	Leechers *PeerMap `json:"leechers"`

	Snatches       uint64  `json:"snatches"`
	UpMultiplier   float64 `json:"upMultiplier"`
	DownMultiplier float64 `json:"downMultiplier"`
	LastAction     int64   `json:"lastAction"`

	Info *TorrentInfo `json:"info"`
}

// PeerCount returns the total number of peers connected on this Torrent.
func (t *Torrent) PeerCount() int {
	return t.Seeders.Len() + t.Leechers.Len()
}

// User is a registered user for private trackers.
type User struct {
	ID             uint64  `json:"id"`
	Passkey        string  `json:"passkey"`
	Username       string  `json:"username"`
	Cred           string  `json:"credential"`
	UpMultiplier   float64 `json:"upMultiplier"`
	DownMultiplier float64 `json:"downMultiplier"`
}

// Announce is an Announce by a Peer.
type Announce struct {
	Config *config.Config `json:"config"`

	Compact    bool     `json:"compact"`
	Downloaded uint64   `json:"downloaded"`
	Event      string   `json:"event"`
	Infohash   string   `json:"infohash"`
	Dest       Endpoint `json:"-"`
	Port       uint16   `json:"port"`
	Left       uint64   `json:"left"`
	NumWant    int      `json:"numwant"`
	Passkey    string   `json:"passkey"`
	PeerID     string   `json:"peer_id"`
	Uploaded   uint64   `json:"uploaded"`

	Torrent *Torrent `json:"-"`
	User    *User    `json:"-"`
	Peer    *Peer    `json:"-"`
}

// ClientID returns the part of a PeerID that identifies a Peer's client
// software.
func (a *Announce) ClientID() (clientID string) {
	length := len(a.PeerID)
	if length >= 6 {
		if a.PeerID[0] == '-' {
			if length >= 7 {
				clientID = a.PeerID[1:7]
			}
		} else {
			clientID = a.PeerID[:6]
		}
	}

	return
}

// BuildPeer creates the Peer representation of an Announce. When provided nil
// for the user or torrent parameter, it creates a Peer{UserID: 0} or
// Peer{TorrentID: 0}, respectively.
func (a *Announce) BuildPeer(u *User, t *Torrent) (err error) {
	a.Peer = &Peer{
		ID:           a.PeerID,
		Uploaded:     a.Uploaded,
		Downloaded:   a.Downloaded,
		Left:         a.Left,
		LastAnnounce: time.Now().Unix(),
	}
	copy(a.Peer.Dest[:], a.Dest.Dest[:])
	a.Peer.Port = a.Port

	if t != nil {
		a.Peer.TorrentID = t.ID
		a.Torrent = t
	}

	if u != nil {
		a.Peer.UserID = u.ID
		a.User = u
	}

	return
}

// AnnounceDelta contains the changes to a Peer's state. These changes are
// recorded by the backend driver.
type AnnounceDelta struct {
	Peer    *Peer
	Torrent *Torrent
	User    *User

	// Created is true if this announce created a new peer or changed an existing
	// peer's address
	Created bool
	// Snatched is true if this announce completed the download
	Snatched bool

	// Uploaded contains the upload delta for this announce, in bytes
	Uploaded    uint64
	RawUploaded uint64

	// Downloaded contains the download delta for this announce, in bytes
	Downloaded    uint64
	RawDownloaded uint64
}

// AnnounceResponse contains the information needed to fulfill an announce.
type AnnounceResponse struct {
	Announce              *Announce
	Complete, Incomplete  int
	Interval, MinInterval time.Duration
	Peers                 PeerList

	Compact bool
}

// Scrape is a Scrape by a Peer.
type Scrape struct {
	Config *config.Config `json:"config"`

	Passkey    string
	Infohashes []string
}

// ScrapeResponse contains the information needed to fulfill a scrape.
type ScrapeResponse struct {
	Files []*Torrent
}

// TorrentCategory contains all info describing a category of torrents on the index
type TorrentCategory struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"desc"`
}
