package logmon_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

func TestTrafficSupervisor_GeneratesStatsWithTraffic(t *testing.T) {
	// Fill up entries channel with test data.
	numEntries := 3
	entries := make(chan logmon.LogEntry, numEntries)
	stats := make(chan logmon.TrafficStats)

	expectedBytes := 0
	for i := 0; i < numEntries; i++ {
		entry, _ := fixtures.GetOneAtRandom()
		entries <- entry
		expectedBytes += entry.Bytes
	}

	// Run supervisor:
	ctx, cancel := context.WithCancel(context.Background())
	supervisor := givenATrafficSupervisor(50)
	go supervisor.Run(ctx, entries, stats)

	// Ensure produced stats are correct.
	data, ok := <-stats
	require.True(t, ok, "stats channel is open")
	require.Equal(t, numEntries, data.TotalReqs, "stats contemplate all the requests")
	require.Equal(t, expectedBytes, data.Bytes, "stats sumps up all bytes transferred")

	// Validate shutdown - output channel ought to be closed:
	cancel()
	_, ok = <-stats
	require.False(t, ok, "stats channel is closed after context cancellation")
}

func TestTrafficSupervisor_GeneratesStatsWithContinuousTraffic(t *testing.T) {
	entries := make(chan logmon.LogEntry)
	stats := make(chan logmon.TrafficStats)

	// Simulate a continuous traffic stream - generate at most `maxEntries` entries:
	maxEntries := 10000
	ctx, cancel := context.WithCancel(context.Background())
	go givenContinuousLogEntryWrites(ctx, entries, maxEntries)

	// Run a producer that crunches stats every 50ms:
	supervisor := givenATrafficSupervisor(50)
	go supervisor.Run(ctx, entries, stats)

	// Capture all generated stats and keep track of the registered requests:
	captured := 0
	var read logmon.TrafficStats
	ok := true
	for ok && captured < maxEntries {
		read, ok = <-stats
		require.True(t, ok, "stats channel is open")
		require.False(t, equalTrafficStats(read, logmon.NewEmptyTrafficStats()), "traffic stats are not empty")
		captured += read.TotalReqs
	}

	require.Equal(t, maxEntries, captured, "captured request match the generated requests")

	cancel()
}

func givenContinuousLogEntryWrites(ctx context.Context, dst chan<- logmon.LogEntry, maxSend int) {
	count := 0
	for {
		if count >= maxSend {
			return
		}

		entry, _ := fixtures.GetOneAtRandom()
		select {
		case dst <- entry:
			count++
		case <-ctx.Done():
			return
		}
	}
}

func TestTrafficSupervisor_GeneratesEmptyStatsWithoutTraffic(t *testing.T) {
	// Run supervisor:
	ctx, cancel := context.WithCancel(context.Background())
	supervisor := givenATrafficSupervisor(50)
	entries := make(chan logmon.LogEntry)
	stats := make(chan logmon.TrafficStats)
	go supervisor.Run(ctx, entries, stats)

	// Ensure produced stats are correct.
	data, ok := <-stats
	require.True(t, ok, "stats channel is open")
	require.Equal(t, 0, data.TotalReqs, "stats reflect no requests")
	require.Equal(t, 0, data.Bytes, "stats reflect no bytes transferred")

	// Validate shutdown - output channel ought to be closed:
	cancel()
	_, ok = <-stats
	require.False(t, ok, "stats channel is closed after context cancellation")
}

func TestTrafficSupervisor_ReturnsWhenInputChannelClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run supervisor:
	supervisor := givenATrafficSupervisor(50)
	entries := make(chan logmon.LogEntry)
	stats := make(chan logmon.TrafficStats)
	go supervisor.Run(ctx, entries, stats)

	// Validate shutdown - output channel ought to be closed:
	close(entries)
	_, ok := <-stats
	require.False(t, ok, "stats channel is closed after input channel is closed")
}

func givenATrafficSupervisor(intervalMs int) logmon.TrafficSupervisor {
	opts := logmon.TrafficSupervisorOpts{RefreshInterval: intervalMs}
	supervisor := logmon.NewTrafficSupervisor(opts)
	return supervisor
}

func TestTrafficStats(t *testing.T) {
	// Base entry and related stats upon which tests are constructed:
	baseEntry := logmon.NewLogEntry(
		"192.168.0.10",
		"-",
		"-",
		time.Now().Format("02/Jan/2006:15:04:05 -0700"),
		"GET",
		"/path",
		"HTTP/2.0",
		200,
		0,
	)
	baseStats := logmon.TrafficStats{
		SectionHits:     map[string]int{"/path": 1},
		MethodHits:      map[string]int{"GET": 1},
		StatusClassHits: map[string]int{"2xx": 1},
		Bytes:           0,
		TotalReqs:       1,
	}

	for name, tc := range map[string]struct {
		LogEntry logmon.LogEntry

		ExpectedStats logmon.TrafficStats
	}{
		"it considers 1xx class status codes": {
			LogEntry:      func() logmon.LogEntry { e := baseEntry; e.StatusCode = 101; return e }(),
			ExpectedStats: func() logmon.TrafficStats { s := baseStats; s.StatusClassHits = map[string]int{"1xx": 1}; return s }(),
		},
		"it considers 2xx class status codes": {
			LogEntry:      func() logmon.LogEntry { e := baseEntry; e.StatusCode = 201; return e }(),
			ExpectedStats: func() logmon.TrafficStats { s := baseStats; s.StatusClassHits = map[string]int{"2xx": 1}; return s }(),
		},
		"it considers 3xx class status codes": {
			LogEntry:      func() logmon.LogEntry { e := baseEntry; e.StatusCode = 301; return e }(),
			ExpectedStats: func() logmon.TrafficStats { s := baseStats; s.StatusClassHits = map[string]int{"3xx": 1}; return s }(),
		},
		"it considers 4xx class status codes": {
			LogEntry:      func() logmon.LogEntry { e := baseEntry; e.StatusCode = 401; return e }(),
			ExpectedStats: func() logmon.TrafficStats { s := baseStats; s.StatusClassHits = map[string]int{"4xx": 1}; return s }(),
		},
		"it considers 5xx class status codes": {
			LogEntry:      func() logmon.LogEntry { e := baseEntry; e.StatusCode = 501; return e }(),
			ExpectedStats: func() logmon.TrafficStats { s := baseStats; s.StatusClassHits = map[string]int{"5xx": 1}; return s }(),
		},
		"it considers empty paths": {
			LogEntry:      func() logmon.LogEntry { e := baseEntry; e.ReqPath = ""; return e }(),
			ExpectedStats: func() logmon.TrafficStats { s := baseStats; s.SectionHits = map[string]int{"/": 1}; return s }(),
		},
		"it considers relative paths without leading slash": {
			LogEntry:      func() logmon.LogEntry { e := baseEntry; e.ReqPath = "abcde"; return e }(),
			ExpectedStats: func() logmon.TrafficStats { s := baseStats; s.SectionHits = map[string]int{"/abcde": 1}; return s }(),
		},
		"it only considers the first part of the path to build the section": {
			LogEntry:      func() logmon.LogEntry { e := baseEntry; e.ReqPath = "a/bb/ccc"; return e }(),
			ExpectedStats: func() logmon.TrafficStats { s := baseStats; s.SectionHits = map[string]int{"/a": 1}; return s }(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			stats := logmon.NewEmptyTrafficStats()
			stats.Update(tc.LogEntry)
			require.True(t, equalTrafficStats(tc.ExpectedStats, stats))
		})
	}
}
