package logmon

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sync"
)

// MonitorOpts defines the options required to build a Monitor.
type MonitorOpts struct {
	LogFilePath     string
	RefreshInterval int
	AlertThreshold  int
	AlertWindow     int
}

// Monitor is a log monitor composed of:
// - a file watcher which detects changes in the log file and produces a stream of LogEntry
// - a traffic supervisor which consumes the stream of LogEntry and produces a stream of TrafficStats
// - an alert supervisor which consumes the stream of TrafficStats and produces a stream of ThresholdAlert
// - an UI which displays information consumed from the TrafficStats and ThresholdAlert streams
type Monitor struct {
	fileWatcher LogEntryProducer
	traffic     TrafficSupervisor
	alert       AlertSupervisor
	ui          UI
}

// NewMonitor creates the Monitor type.
func NewMonitor(opts MonitorOpts) *Monitor {
	producer := NewLogEntryProducer(
		ProducerOpts{
			opts.LogFilePath,
			io.SeekEnd,
			log.New(ioutil.Discard, "", 0),
			NewW3CommonLogParser(),
		},
	)

	traffic := NewTrafficSupervisor(
		TrafficSupervisorOpts{opts.RefreshInterval * 1000 /* in milliseconds */},
	)

	alert := NewAlertsSupervisor(
		AlertSupervisorOpts{opts.AlertThreshold, opts.RefreshInterval, opts.AlertWindow},
	)

	ui := NewUI(
		UIOpts{Refresh: opts.RefreshInterval, AlertThreshold: opts.AlertThreshold, AlertWindow: opts.AlertWindow},
	)

	return &Monitor{fileWatcher: producer, traffic: traffic, alert: alert, ui: ui}
}

// Run executes all the components of the log monitor.
// It orchestrates the setup, error handling and execution of the components.
// The file watcher, traffic supervisor and alert supervisor run on their own goroutine.
// The UI runs on the main goroutine and captures interruption signals.
// On shutdown, it waits for all components to stop before exiting.
func (m Monitor) Run(parentCtx context.Context) error {
	cleanupProducer, err := m.fileWatcher.Setup()
	if err != nil {
		return fmt.Errorf("setup file watcher: %w", err)
	}
	defer cleanupProducer()

	cleanupUI, err := m.ui.Setup()
	if err != nil {
		return fmt.Errorf("setup ui: %w", err)
	}
	defer cleanupUI()

	ctx, cancel := context.WithCancel(parentCtx)
	var wg sync.WaitGroup

	// Launch each component on a different goroutine:
	logEntries := m.launchLogEntryProducer(ctx, &wg)
	statsForAlerts, statsForUI := m.launchTrafficSupervisor(ctx, &wg, logEntries)
	alerts := m.launchAlertManager(ctx, &wg, statsForAlerts)

	// Launch the UI in the main goroutine.
	// UI loops until an interrupt signal is captured.
	m.ui.Run(ctx, statsForUI, alerts)

	// On shutdown, wait for all components to stop before exiting.
	cancel()
	wg.Wait()

	return nil
}

func (m Monitor) launchAlertManager(ctx context.Context, wg *sync.WaitGroup, statsForAlerts chan TrafficStats) chan ThresholdAlert {
	alerts := make(chan ThresholdAlert)
	wg.Add(1)
	go func() {
		m.alert.Run(ctx, statsForAlerts, alerts)
		wg.Done()
	}()
	return alerts
}

func (m Monitor) launchTrafficSupervisor(ctx context.Context, wg *sync.WaitGroup, logEntries chan LogEntry) (chan TrafficStats, chan TrafficStats) {
	trafficStats := make(chan TrafficStats)
	wg.Add(1)
	go func() {
		m.traffic.Run(ctx, logEntries, trafficStats)
		wg.Done()
	}()

	return broadcastTrafficStats(ctx, trafficStats)
}

func (m Monitor) launchLogEntryProducer(ctx context.Context, wg *sync.WaitGroup) chan LogEntry {
	logEntries := make(chan LogEntry)
	wg.Add(1)
	go func() {
		m.fileWatcher.Run(ctx, logEntries)
		wg.Done()
	}()
	return logEntries
}

// broadcastTrafficStats broadcasts the messages from the input channel into two output channels.
func broadcastTrafficStats(ctx context.Context, input chan TrafficStats) (chan TrafficStats, chan TrafficStats) {
	output1 := make(chan TrafficStats)
	output2 := make(chan TrafficStats)
	go func() {
		for {
			select {
			case msg := <-input:
				output1 <- msg
				output2 <- msg
			case <-ctx.Done():
				return
			}
		}
	}()
	return output1, output2
}
