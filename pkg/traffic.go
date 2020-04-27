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
			case tick := <-ticker.C:
				wg.Add(1)
				go t.produceStats(stats, tick, &wg)
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
	t.mutex.Lock()
	t.registry.PushFront(entry)
	t.mutex.Unlock()
}

// produceStats considers entries within a time window.
// it starts consuming the oldest entry and continues up to the given time limit.
// every consumed entry is removed from the local storage.
func (t *trafficSupervisor) produceStats(statsBus chan<- TrafficStats, maxTimeLimit time.Time, wg *sync.WaitGroup) {
	interval := t.extractIntervalFromStorage(maxTimeLimit)

	stats := NewEmptyTrafficStats()
	for _, e := range interval {
		stats.Update(e)
	}

	statsBus <- stats
	wg.Done()
}

func (t *trafficSupervisor) extractIntervalFromStorage(maxTimeLimit time.Time) []LogEntry {
	var interval []LogEntry
	var e, prev *list.Element
	var logEntry LogEntry

	t.mutex.Lock()
	e = t.registry.Back()
	for e != nil {
		logEntry = e.Value.(LogEntry)
		if logEntry.CreatedAt.After(maxTimeLimit) {
			break
		}

		prev = e.Prev()
		interval = append(interval, t.registry.Remove(e).(LogEntry))
		e = prev
	}
	t.mutex.Unlock()

	return interval
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
