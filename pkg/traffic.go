package logmon

import (
	"container/list"
	"context"
	"log"
	"regexp"
	"sync"
	"time"
)

// TrafficSupervisor consumes log entries and produces traffic stats.
type TrafficSupervisor interface {
	Run(ctx context.Context, entries <-chan LogEntry, stats chan<- TrafficStats)
}

// NewTrafficSupervisor creates a TrafficSupervisor.
func NewTrafficSupervisor(opts TrafficSupervisorOpts) TrafficSupervisor {
	return &trafficSupervisor{
		refreshInterval: time.Duration(opts.RefreshInterval) * time.Millisecond,
		entriesBuffer:   list.New(),
	}
}

// TrafficSupervisorOpts defines the options required to build a TrafficSupervisor.
type TrafficSupervisorOpts struct {
	RefreshInterval int
}

// traficSupervisor implements the TrafficSupervisor interface.
// It stores the log entries of the current refresh interval in a linked-list.
type trafficSupervisor struct {
	entriesBuffer   *list.List
	refreshInterval time.Duration
}

// Run consumes log entries and produces traffic stats.
// Every log entry received is stored in a linked-list. Only the current interval is kept in the list.
// Traffic stats generation is scheduled based on the refresh interval.
// On every refresh interval tick, the current buffer of log entries is used to generate the stats.
// The log entries buffer is replaced with an empty list that will store the entries of the next interval.
func (t *trafficSupervisor) Run(ctx context.Context, entries <-chan LogEntry, stats chan<- TrafficStats) {
	var wg sync.WaitGroup
	ticker := time.NewTicker(t.refreshInterval)

LOOP:
	for {
		select {
		case entry, ok := <-entries:
			if !ok {
				break LOOP
			}

			t.entriesBuffer.PushFront(entry)
		case <-ticker.C:
			// Keep a reference to the current list of entries to compute stats.
			// Create a new list for the next tick.
			interval := t.entriesBuffer
			t.entriesBuffer = list.New()

			wg.Add(1)
			go t.produceStats(&wg, interval, stats)
		case <-ctx.Done():
			break LOOP
		}
	}

	wg.Wait() // Wait for any goroutine that might be generating stats.
	log.Printf("clean up: close stats channel & ticker")
	close(stats)
	ticker.Stop()
}

// produceStats considers entries within a time window.
// it starts consuming the oldest entry and continues up to the given time limit.
// every consumed entry is freed.
func (t *trafficSupervisor) produceStats(wg *sync.WaitGroup, interval *list.List, statsC chan<- TrafficStats) {
	stats := NewEmptyTrafficStats()

	count := interval.Len()
	var e, prev *list.Element
	e = interval.Back()
	for e != nil {
		stats.Update(e.Value.(LogEntry))

		prev = e.Prev()
		interval.Remove(e)
		e = prev
	}

	log.Printf("send stats from %d entries: %v", count, stats)
	statsC <- stats
	wg.Done()
}

// TrafficStats defines the stats store for the traffic during an interval.
type TrafficStats struct {
	SectionHits     map[string]int
	MethodHits      map[string]int
	StatusClassHits map[string]int
	Bytes           int
	TotalReqs       int
}

// NewEmptyTrafficStats creates an empty TrafficStats.
func NewEmptyTrafficStats() TrafficStats {
	return TrafficStats{
		SectionHits:     make(map[string]int),
		MethodHits:      make(map[string]int),
		StatusClassHits: make(map[string]int),
	}
}

// Update updates the traffic stats with a LogEntry.
func (s *TrafficStats) Update(entry LogEntry) {
	s.SectionHits[s.parseSection(entry.ReqPath)]++
	s.MethodHits[entry.ReqMethod]++
	s.StatusClassHits[s.parseStatusClass(entry.StatusCode)]++
	s.Bytes += entry.Bytes
	s.TotalReqs++
}

// parseSection finds the section in the given URL path.
func (s *TrafficStats) parseSection(path string) string {
	if len(path) < 1 || path[0] != '/' {
		path = "/" + path
	}

	rx := regexp.MustCompile(`^/[^/]*`)
	return rx.FindString(path)
}

// parseStatusClass classifies HTTP status codes into classes.
func (s *TrafficStats) parseStatusClass(code int) string {
	if code < 200 {
		return "1xx"
	}
	if code >= 200 && code <= 299 {
		return "2xx"
	}
	if code >= 300 && code <= 399 {
		return "3xx"
	}
	if code >= 400 && code <= 499 {
		return "4xx"
	}
	return "5xx"
}
