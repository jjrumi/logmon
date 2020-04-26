package logmon

import (
	"container/list"
	"context"
	"log"
	"regexp"
	"sync"
	"time"
)

// Reads LogEntry and produces TrafficStats
type TrafficSupervisor interface {
	Run(ctx context.Context, entries <-chan LogEntry) (<-chan TrafficStats, func())
}

func NewTrafficSupervisor(opts SupervisorOpts) TrafficSupervisor {
	return &trafficSupervisor{
		refreshInterval: time.Duration(opts.RefreshInterval) * time.Second,
		registry:        list.New(),
		mutex:           &sync.Mutex{},
	}
}

type SupervisorOpts struct {
	RefreshInterval int
}

type trafficSupervisor struct {
	registry        *list.List
	mutex           *sync.Mutex
	refreshInterval time.Duration
}

func (t *trafficSupervisor) Run(ctx context.Context, entries <-chan LogEntry) (<-chan TrafficStats, func()) {
	stats := make(chan TrafficStats, 1)
	ticker := time.NewTicker(t.refreshInterval)

	go func() {
	LOOP:
		for {
			select {
			case entry, ok := <-entries:
				if !ok {
					log.Printf("producer channel closed")
					break LOOP
				}

				// TODO: Benchmark under stress. Better to register in a new goroutine ??
				t.registerEntry(entry)
			case tick := <-ticker.C:
				go t.produceStats(stats, tick)
			case <-ctx.Done():
				break LOOP
			}
		}
		close(stats)
	}()

	cleanup := func() {
		// TODO: Empty remaining entries from registry?
	}
	return stats, cleanup
}

func (t *trafficSupervisor) registerEntry(entry LogEntry) {
	t.mutex.Lock()
	t.registry.PushFront(entry)
	t.mutex.Unlock()
}

func (t *trafficSupervisor) produceStats(statsBus chan<- TrafficStats, maxTimeLimit time.Time) {
	var e, prev *list.Element
	var logEntry LogEntry
	stats := NewTrafficStats()

	t.mutex.Lock()
	e = t.registry.Back()
	t.mutex.Unlock()
	for e != nil {
		t.mutex.Lock()
		logEntry = e.Value.(LogEntry)
		if logEntry.Date.After(maxTimeLimit) {
			t.mutex.Unlock()
			break
		}

		prev = e.Prev()
		t.registry.Remove(e)
		t.mutex.Unlock()
		e = prev

		stats.Update(logEntry)
	}

	statsBus <- stats
}

type TrafficStats struct {
	sectionHits     map[string]int
	methodHits      map[string]int
	statusClassHits map[string]int
	Bytes           int
	TotalReqs       int
}

func NewTrafficStats() TrafficStats {
	return TrafficStats{
		sectionHits:     make(map[string]int),
		methodHits:      make(map[string]int),
		statusClassHits: make(map[string]int),
	}
}

func (s *TrafficStats) Update(entry LogEntry) {
	s.sectionHits[s.parseSection(entry.ReqPath)]++
	s.methodHits[entry.ReqMethod]++
	s.statusClassHits[s.parseStatusClass(entry.StatusCode)]++
	s.Bytes += entry.Bytes
	s.TotalReqs++
}

func (s *TrafficStats) parseSection(path string) string {
	if path[0] != '/' {
		path = "/" + path
	}

	rx := regexp.MustCompile(`^/[^/]*`)
	return rx.FindString(path)
}

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
