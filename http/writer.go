// Copyright 2015 The Chihaya Authors. All rights reserved.
// Use of this source code is governed by the BSD 2-Clause license,
// which can be found in the LICENSE file.

package http

import (
	"bytes"
	"net/http"

	"github.com/chihaya/bencode"
	"github.com/majestrate/chihaya/tracker/models"
)

// Writer implements the tracker.Writer interface for the HTTP protocol.
type Writer struct {
	http.ResponseWriter
}

// WriteError writes a bencode dict with a failure reason.
func (w *Writer) WriteError(err error) error {
	bencoder := bencode.NewEncoder(w)

	w.Header().Set("Content-Type", "text/plain")
	return bencoder.Encode(bencode.Dict{
		"failure reason": err.Error(),
	})
}

// WriteAnnounce writes a bencode dict representation of an AnnounceResponse.
func (w *Writer) WriteAnnounce(res *models.AnnounceResponse) error {
	dict := bencode.Dict{
		"complete":     res.Complete,
		"incomplete":   res.Incomplete,
		"interval":     res.Interval,
		"min interval": res.MinInterval,
		"compact":      1,
	}

	dict["peers"] = compactPeers(res.Peers)

	w.Header().Set("Content-Type", "text/plain")
	bencoder := bencode.NewEncoder(w)
	return bencoder.Encode(dict)
}

// WriteScrape writes a bencode dict representation of a ScrapeResponse.
func (w *Writer) WriteScrape(res *models.ScrapeResponse) error {
	dict := bencode.Dict{
		"files": filesDict(res.Files),
	}

	w.Header().Set("Content-Type", "text/plain")
	bencoder := bencode.NewEncoder(w)
	return bencoder.Encode(dict)
}

func compactPeers(peers models.PeerList) []byte {
	var compactPeers bytes.Buffer
	for _, peer := range peers {
		compactPeers.Write(peer.Dest[:])
	}
	return compactPeers.Bytes()
}

func filesDict(torrents []*models.Torrent) bencode.Dict {
	d := bencode.NewDict()
	for _, torrent := range torrents {
		d[torrent.Infohash] = torrentDict(torrent)
	}
	return d
}

func torrentDict(torrent *models.Torrent) bencode.Dict {
	return bencode.Dict{
		"complete":   torrent.Seeders.Len(),
		"incomplete": torrent.Leechers.Len(),
		"downloaded": torrent.Snatches,
	}
}
