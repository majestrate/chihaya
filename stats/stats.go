// Copyright 2015 The Chihaya Authors. All rights reserved.
// Use of this source code is governed by the BSD 2-Clause license,
// which can be found in the LICENSE file.

// Package stats implements a means of tracking processing statistics for a
// BitTorrent tracker.
package stats

import (
	"time"

	"github.com/pushrax/faststats"
	"github.com/pushrax/flatjson"

	"github.com/majestrate/chihaya/config"
)

const (
	Announce = iota
	Scrape

	Completed
	NewLeech
	DeletedLeech
	ReapedLeech
	NewSeed
	DeletedSeed
	ReapedSeed

	NewTorrent
	DeletedTorrent
	ReapedTorrent

	AcceptedConnection
	ClosedConnection

	HandledRequest
	ErroredRequest
	ClientError

	ResponseTime
)

// DefaultStats is a default instance of stats tracking that uses an unbuffered
// channel for broadcasting events unless specified otherwise via a command
// line flag.
var DefaultStats *Stats

type PeerClassStats struct {
	Current int64  // Current peer count.
	Joined  uint64 // Peers that announced.
	Left    uint64 // Peers that paused or stopped.
	Reaped  uint64 // Peers cleaned up after inactivity.
}

type PeerStats struct {
	PeerClassStats `json:"Peers"` // Stats for all peers.

	Seeds     PeerClassStats // Stats for seeds only.
	Completed uint64         // Number of transitions from leech to seed.
}

type PercentileTimes struct {
	P50 *faststats.Percentile
	P90 *faststats.Percentile
	P95 *faststats.Percentile
}

type Stats struct {
	Started time.Time // Time at which Chihaya was booted.

	OpenConnections     int64  `json:"connectionsOpen"`
	ConnectionsAccepted uint64 `json:"connectionsAccepted"`
	BytesTransmitted    uint64 `json:"bytesTransmitted"`

	GoRoutines int `json:"runtimeGoRoutines"`

	RequestsHandled uint64 `json:"requestsHandled"`
	RequestsErrored uint64 `json:"requestsErrored"`
	ClientErrors    uint64 `json:"requestsBad"`
	ResponseTime    PercentileTimes

	Announces uint64 `json:"trackerAnnounces"`
	Scrapes   uint64 `json:"trackerScrapes"`

	TorrentsSize    uint64 `json:"torrentsSize"`
	TorrentsAdded   uint64 `json:"torrentsAdded"`
	TorrentsRemoved uint64 `json:"torrentsRemoved"`
	TorrentsReaped  uint64 `json:"torrentsReaped"`

	Peers PeerStats `json:"peers`

	*MemStatsWrapper `json:",omitempty"`

	events             chan int
	peerEvents         chan int
	responseTimeEvents chan time.Duration
	recordMemStats     <-chan time.Time

	flattened flatjson.Map
}

func New(cfg config.StatsConfig) *Stats {
	s := &Stats{
		Started: time.Now(),
		events:  make(chan int, cfg.BufferSize),

		GoRoutines: 0,

		peerEvents:         make(chan int, cfg.BufferSize),
		responseTimeEvents: make(chan time.Duration, cfg.BufferSize),

		ResponseTime: PercentileTimes{
			P50: faststats.NewPercentile(0.5),
			P90: faststats.NewPercentile(0.9),
			P95: faststats.NewPercentile(0.95),
		},
	}

	if cfg.IncludeMem {
		s.MemStatsWrapper = NewMemStatsWrapper(cfg.VerboseMem)
		s.recordMemStats = time.NewTicker(cfg.MemUpdateInterval.Duration).C
	}

	s.flattened = flatjson.Flatten(s)
	go s.handleEvents()
	return s
}

func (s *Stats) Flattened() flatjson.Map {
	return s.flattened
}

func (s *Stats) Close() {
	close(s.events)
}

func (s *Stats) Uptime() time.Duration {
	return time.Since(s.Started)
}

func (s *Stats) RecordEvent(event int) {
	s.events <- event
}

func (s *Stats) RecordPeerEvent(event int) {
	s.peerEvents <- event
}

func (s *Stats) RecordTiming(event int, duration time.Duration) {
	switch event {
	case ResponseTime:
		s.responseTimeEvents <- duration
	default:
		panic("stats: RecordTiming called with an unknown event")
	}
}

func (s *Stats) handleEvents() {
	for {
		select {
		case event := <-s.events:
			s.handleEvent(event)

		case event := <-s.peerEvents:
			s.handlePeerEvent(&s.Peers, event)

		case duration := <-s.responseTimeEvents:
			f := float64(duration) / float64(time.Millisecond)
			s.ResponseTime.P50.AddSample(f)
			s.ResponseTime.P90.AddSample(f)
			s.ResponseTime.P95.AddSample(f)

		case <-s.recordMemStats:
			s.MemStatsWrapper.Update()
		}
	}
}

func (s *Stats) handleEvent(event int) {
	switch event {
	case Announce:
		s.Announces++

	case Scrape:
		s.Scrapes++

	case NewTorrent:
		s.TorrentsAdded++
		s.TorrentsSize++

	case DeletedTorrent:
		s.TorrentsRemoved++
		s.TorrentsSize--

	case ReapedTorrent:
		s.TorrentsReaped++
		s.TorrentsSize--

	case AcceptedConnection:
		s.ConnectionsAccepted++
		s.OpenConnections++

	case ClosedConnection:
		s.OpenConnections--

	case HandledRequest:
		s.RequestsHandled++

	case ClientError:
		s.ClientErrors++

	case ErroredRequest:
		s.RequestsErrored++

	default:
		panic("stats: RecordEvent called with an unknown event")
	}
}

func (s *Stats) handlePeerEvent(ps *PeerStats, event int) {
	switch event {
	case Completed:
		ps.Completed++
		ps.Seeds.Current++

	case NewLeech:
		ps.Joined++
		ps.Current++

	case DeletedLeech:
		ps.Left++
		ps.Current--

	case ReapedLeech:
		ps.Reaped++
		ps.Current--

	case NewSeed:
		ps.Seeds.Joined++
		ps.Seeds.Current++
		ps.Joined++
		ps.Current++

	case DeletedSeed:
		ps.Seeds.Left++
		ps.Seeds.Current--
		ps.Left++
		ps.Current--

	case ReapedSeed:
		ps.Seeds.Reaped++
		ps.Seeds.Current--
		ps.Reaped++
		ps.Current--

	default:
		panic("stats: RecordPeerEvent called with an unknown event")
	}
}

// RecordEvent broadcasts an event to the default stats queue.
func RecordEvent(event int) {
	if DefaultStats != nil {
		DefaultStats.RecordEvent(event)
	}
}

// RecordPeerEvent broadcasts a peer event to the default stats queue.
func RecordPeerEvent(event int) {
	if DefaultStats != nil {
		DefaultStats.RecordPeerEvent(event)
	}
}

// RecordTiming broadcasts a timing event to the default stats queue.
func RecordTiming(event int, duration time.Duration) {
	if DefaultStats != nil {
		DefaultStats.RecordTiming(event, duration)
	}
}
