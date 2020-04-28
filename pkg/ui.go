package logmon

import (
	"context"
	"fmt"
	"log"
)

func NewUI() UI {
	return UI{}
}

type UI struct {
}

func (u UI) Run(ctx context.Context, stats <-chan TrafficStats, alerts <-chan ThresholdAlert) {
LOOP:
	for {
		select {
		case s, ok := <-stats:
			if !ok {
				log.Printf("stats channel closed")
				break LOOP
			}

			fmt.Printf("%v\n", s)
		case a, ok := <-alerts:
			if !ok {
				log.Printf("alerts channel closed")
				break LOOP
			}

			if a.Open {
				fmt.Printf("High traffic generated an alert - hits = %v req/s, triggered at %v\n", a.Hits, a.Time)
			} else {
				fmt.Printf("(Recovered) High traffic alert. Current hits = %v req/s, triggered at %v\n", a.Hits, a.Time)
			}
		case <-ctx.Done():
			break LOOP
		}
	}
}
