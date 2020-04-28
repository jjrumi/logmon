package logmon

import (
	"container/list"
	"context"
	"time"
)

type ThresholdAlert struct {
	Open bool // true: Unresolved alert; false: Recovered alert.
	Hits float64
	Time time.Time
}

type AlertSupervisor interface {
	Run(ctx context.Context, stats <-chan TrafficStats, alerts chan<- ThresholdAlert)
}

func NewAlertsSupervisor(opts AlertSupervisorOpts) AlertSupervisor {
	return &alertSupervisor{
		stats:     list.New(),
		capacity:  opts.AlertWindow / opts.RefreshInterval, // Store as many stats as intervals fit in the monitoring window.
		ongoing:   false,
		threshold: opts.AlertThreshold,
		window:    opts.AlertWindow,
	}
}

type AlertSupervisorOpts struct {
	AlertThreshold  int
	RefreshInterval int
	AlertWindow     int
}

type alertSupervisor struct {
	stats        *list.List // Buffer to store all the stats within the alert window.
	capacity     int        // Number of stats to store.
	ongoing      bool       // Is the alert active?
	reqsInWindow int        // Counter for requests within the alert window.
	threshold    int        // ThresholdAlert condition, in requests per second.
	window       int        // Time alert window, in seconds.
}

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

	close(alerts)
}

func (a *alertSupervisor) trackAlerts(s TrafficStats, alerts chan<- ThresholdAlert) {
	// Keep track of the stats within the alert window:
	a.stats.PushFront(s)
	a.reqsInWindow += s.TotalReqs

	// Remove old stats:
	if a.stats.Len() > a.capacity {
		last := a.stats.Back()
		oldStat := a.stats.Remove(last)
		a.reqsInWindow -= oldStat.(TrafficStats).TotalReqs
	}

	// Check alert condition:
	reqsPerSec := float64(a.reqsInWindow) / float64(a.window)

	if a.ongoing {
		if reqsPerSec < float64(a.threshold) {
			a.ongoing = false
			alerts <- ThresholdAlert{Open: false, Hits: reqsPerSec, Time: time.Now()}
		}
	} else {
		if reqsPerSec > float64(a.threshold) {
			a.ongoing = true
			alerts <- ThresholdAlert{Open: true, Hits: reqsPerSec, Time: time.Now()}
		}
	}
}
