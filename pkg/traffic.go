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
	Run(ctx context.Context, entries <-chan LogEntry) <-chan TrafficStats
}

func NewTrafficSupervisor(opts SupervisorOpts) TrafficSupervisor {
	return &trafficSupervisor{
		refreshInterval: time.Duration(opts.RefreshInterval) * time.Millisecond,
		registry:        list.New(),
	}
}

type SupervisorOpts struct {
	RefreshInterval int
}

type trafficSupervisor struct {
	registry        *list.List // All read/write is done within a single goroutine, inside Run()
	refreshInterval time.Duration
}

func (t *trafficSupervisor) Run(ctx context.Context, entries <-chan LogEntry) <-chan TrafficStats {
	stats := make(chan TrafficStats, 1)
	ticker := time.NewTicker(t.refreshInterval)

	go func() {
		var wg sync.WaitGroup
	LOOP:
		for {
			select {
			case entry, ok := <-entries:
				if !ok {
					log.Printf("producer channel closed")
					break LOOP
				}

				t.registerEntry(entry)
			case <-ticker.C:
				// Keep a reference to the current list of entries.
				// Create a new list for the next tick.
				interval := t.registry
				t.registry = list.New()

				wg.Add(1)
				go t.produceStats(interval, stats, &wg)
			case <-ctx.Done():
				break LOOP
			}
		}

		wg.Wait()
		close(stats)
	}()

	return stats
}

func (t *trafficSupervisor) registerEntry(entry LogEntry) {
	t.registry.PushFront(entry)
}

// produceStats considers entries within a time window.
// it starts consuming the oldest entry and continues up to the given time limit.
// every consumed entry is removed from the local storage.
func (t *trafficSupervisor) produceStats(interval *list.List, statsBus chan<- TrafficStats, wg *sync.WaitGroup) {
	stats := NewEmptyTrafficStats()

	var e, prev *list.Element
	e = interval.Back()
	for e != nil {
		stats.Update(e.Value.(LogEntry))

		prev = e.Prev()
		interval.Remove(e)
		e = prev
	}

	statsBus <- stats
	wg.Done()
}

type TrafficStats struct {
	SectionHits     map[string]int
	MethodHits      map[string]int
	StatusClassHits map[string]int
	Bytes           int
	TotalReqs       int
}

func NewEmptyTrafficStats() TrafficStats {
	return TrafficStats{
		SectionHits:     make(map[string]int),
		MethodHits:      make(map[string]int),
		StatusClassHits: make(map[string]int),
	}
}

func (s *TrafficStats) Update(entry LogEntry) {
	s.SectionHits[s.parseSection(entry.ReqPath)]++
	s.MethodHits[entry.ReqMethod]++
	s.StatusClassHits[s.parseStatusClass(entry.StatusCode)]++
	s.Bytes += entry.Bytes
	s.TotalReqs++
}

func (s *TrafficStats) parseSection(path string) string {
	if len(path) < 1 || path[0] != '/' {
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
