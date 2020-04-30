package logmon

import (
	"context"
	"fmt"
	"log"
	"time"
)

type UIOpts struct {
	Refresh        int
	AlertThreshold int
	AlertWindow    int
}

func NewUI(opts UIOpts) UI {
	return UI{refresh: opts.Refresh, alertThreshold: opts.AlertThreshold, alertWindow: opts.AlertWindow}
}

type UI struct {
	refresh        int
	alertThreshold int
	alertWindow    int
	stats          TrafficStats
	alert          ThresholdAlert
}

func (u UI) Run(ctx context.Context, stats <-chan TrafficStats, alerts <-chan ThresholdAlert) {
	u.renderHeader()
	fmt.Println("waiting for inputs...")

LOOP:
	for {
		select {
		case s, ok := <-stats:
			log.Printf("stats: %v", s)
			if !ok {
				log.Printf("stats channel closed")
				break LOOP
			}

			u.stats = s

			u.renderHeader()
			u.renderAlerts()
			u.renderStats()
		case a, ok := <-alerts:
			log.Printf("alert: %v", a)
			if !ok {
				log.Printf("alerts channel closed")
				break LOOP
			}

			u.alert = a

			u.renderHeader()
			u.renderAlerts()
			u.renderStats()
		case <-ctx.Done():
			break LOOP
		}
	}
}

func (u UI) renderHeader() {
	fmt.Print("\033[H\033[2J")
	fmt.Printf("ACCESS LOG MONITOR\n==================\n")
	fmt.Printf(
		"** Refresh interval: %v (seconds) - Alert threshold: %v (seconds) - Alert window: %v (seconds)\n",
		u.refresh,
		u.alertThreshold,
		u.alertWindow,
	)
	fmt.Printf("** Current time: %v\n\n", time.Now().Format(time.RFC1123))
}

func (u UI) renderAlerts() {
	fmt.Printf("Alerts\n------\n")
	if u.alert == (ThresholdAlert{}) {
		fmt.Printf("no alerts registered\n")
	} else {
		if u.alert.Open {
			fmt.Printf("High traffic generated an alert - hits = %v req/s, triggered at %v\n", u.alert.Hits, u.alert.Time)
		} else {
			fmt.Printf("(Recovered) High traffic alert. Current hits = %v req/s, triggered at %v\n", u.alert.Hits, u.alert.Time)
		}
		fmt.Printf("alerts: %v\n", u.alert)
	}
	fmt.Println()
}

func (u UI) renderStats() {
	fmt.Printf("Stats\n-----\n")
	if u.stats.TotalReqs < 1 {
		fmt.Printf("waiting for log entries...\n")
	} else {
		fmt.Printf("Total requests: %v\n", u.stats.TotalReqs)
		fmt.Printf("Bytes tranferred: %v\n", u.stats.Bytes)
		fmt.Println("Requests per section")
		for section, hits := range u.stats.SectionHits {
			fmt.Printf("%v: %v\n", section, hits)
		}
		fmt.Println()

		fmt.Println("Requests per HTTP method")
		for method, hits := range u.stats.MethodHits {
			fmt.Printf("%v: %v\n", method, hits)
		}
		fmt.Println()

		fmt.Println("Response status codes")
		for status, hits := range u.stats.StatusClassHits {
			fmt.Printf("%v: %v", status, hits)
		}
		fmt.Println()
	}
}
