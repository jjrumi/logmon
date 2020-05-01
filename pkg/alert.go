package logmon

import (
	"container/list"
	"context"
	"log"
	"time"
)

// ThresholdAlert defines an alert.
type ThresholdAlert struct {
	Open bool // true: Unresolved alert; false: Recovered alert.
	Hits float64
	Time time.Time
}

// AlertSupervisor consumes traffic stats and produces alerts.
type AlertSupervisor interface {
	Run(ctx context.Context, stats <-chan TrafficStats, alerts chan<- ThresholdAlert)
}

// NewAlertsSupervisor creates an AlertSupervisor.
func NewAlertsSupervisor(opts AlertSupervisorOpts) AlertSupervisor {
	return &alertSupervisor{
		statsBuffer: list.New(),
		capacity:    opts.AlertWindow / opts.RefreshInterval, // Store as many stats as intervals fit in the monitoring window.
		ongoing:     false,
		threshold:   opts.AlertThreshold,
		window:      opts.AlertWindow,
	}
}

// AlertSupervisorOpts defines the options required to build an AlertSupervisor.
type AlertSupervisorOpts struct {
	AlertThreshold  int
	RefreshInterval int
	AlertWindow     int
}

// alertSupervisor implements the AlertSupervisor interface.
// It stores the traffic stats of a monitoring window in a linked-list.
type alertSupervisor struct {
	statsBuffer  *list.List // Buffer to store all the stats within the alert window.
	capacity     int        // Number of stats to store.
	ongoing      bool       // Is the alert active?
	reqsInWindow int        // Counter for requests within the alert window.
	threshold    int        // ThresholdAlert condition, in requests per second.
	window       int        // Alert window, in seconds.
}

// Run consumes traffic stats and produces alerts.
func (a *alertSupervisor) Run(ctx context.Context, stats <-chan TrafficStats, alerts chan<- ThresholdAlert) {
LOOP:
	for {
		select {
		case s, ok := <-stats:
			if !ok {
				break LOOP
			}

			a.trackAlerts(s, alerts)
		case <-ctx.Done():
			break LOOP
		}
	}

	log.Printf("clean up: close alerts channel")
	close(alerts)
}

// trackAlerts updates the storage of stats for the current monitoring window.
// It adds new stats to the front of the list.
// It removes old stats from the back of the list.
// Alert threshold is checked against a pre-calculated value, updated on every new traffic stats.
func (a *alertSupervisor) trackAlerts(s TrafficStats, alerts chan<- ThresholdAlert) {
	// Keep track of the stats within the alert window:
	a.statsBuffer.PushFront(s)
	a.reqsInWindow += s.TotalReqs

	// Remove old stats:
	if a.statsBuffer.Len() > a.capacity {
		last := a.statsBuffer.Back()
		oldStat := a.statsBuffer.Remove(last)
		a.reqsInWindow -= oldStat.(TrafficStats).TotalReqs
	}

	// Check alert condition:
	reqsPerSec := float64(a.reqsInWindow) / float64(a.window)

	if a.ongoing {
		if reqsPerSec < float64(a.threshold) {
			a.ongoing = false
			alert := ThresholdAlert{Open: false, Hits: reqsPerSec, Time: time.Now()}
			log.Printf("close ongoing alert: %v", alert)
			alerts <- alert
		}
	} else {
		if reqsPerSec > float64(a.threshold) {
			a.ongoing = true
			alert := ThresholdAlert{Open: true, Hits: reqsPerSec, Time: time.Now()}
			log.Printf("create alert: %v", alert)
			alerts <- alert
		}
	}
}
