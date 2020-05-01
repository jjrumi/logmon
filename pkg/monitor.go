package logmon

import (
	"context"
	"fmt"
	"io"
	"time"
)

type MonitorOpts struct {
	LogFilePath     string
	RefreshInterval int
	AlertThreshold  int
	AlertWindow     int
}

type Monitor struct {
	fileWatcher LogEntryProducer
	traffic     TrafficSupervisor
	alert       AlertSupervisor
	ui          UI
}

// LogEntry represents a line in a log file that follows a common format as in:
// https://www.w3.org/Daemon/User/Config/Logging.html#common-logfile-format
type LogEntry struct {
	RemoteHost  string    // Remote hostname (or IP number if DNS hostname is not available).
	UserID      string    // The remote logname of the user as in rfc931.
	Username    string    // The username as which the user has authenticated himself.
	Date        time.Time // Date and time of the request.
	ReqMethod   string    // The HTTP request method.
	ReqPath     string    // The HTTP request path.
	ReqProtocol string    // The HTTP request protocol.
	StatusCode  int       // The HTTP status code.
	Bytes       int       // The content-length of the document transferred.
	CreatedAt   time.Time // Time mark of the creation of this type.
}

func NewEmptyLogEntry() LogEntry {
	return LogEntry{CreatedAt: time.Now()}
}
func NewLogEntry(
	host string,
	userID string,
	userName string,
	date time.Time,
	method string,
	path string,
	protocol string,
	status int,
	bytes int,
) LogEntry {
	return LogEntry{
		host,
		userID,
		userName,
		date,
		method,
		path,
		protocol,
		status,
		bytes,
		time.Now(),
	}
}

func NewMonitor(opts MonitorOpts) *Monitor {
	producer := NewLogEntryProducer(
		ProducerOpts{opts.LogFilePath, io.SeekEnd, nil},
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

func (m Monitor) Run(ctx context.Context) error {
	cleanupProducer, err := m.fileWatcher.Setup()
	if err != nil {
		return fmt.Errorf("setup file watcher: %w", err)
	}
	defer cleanupProducer()

	logEntries := make(chan LogEntry)
	go m.fileWatcher.Run(ctx, logEntries)

	trafficStats := make(chan TrafficStats)
	go m.traffic.Run(ctx, logEntries, trafficStats)

	statsForAlerts, statsForUI := broadcastTrafficStats(ctx, trafficStats)

	alerts := make(chan ThresholdAlert)
	go m.alert.Run(ctx, statsForAlerts, alerts)

	err = m.ui.Setup()
	if err != nil {
		return fmt.Errorf("setup ui: %w", err)
	}

	// UI loops until an interrupt signal is captured.
	m.ui.Run(ctx, statsForUI, alerts)

	return nil
}

// broadcastTrafficStats reads stats from the Traffic Supervisor and broadcasts them into two channels.
func broadcastTrafficStats(ctx context.Context, stats chan TrafficStats) (chan TrafficStats, chan TrafficStats) {
	statsForAlerts := make(chan TrafficStats)
	statsForUI := make(chan TrafficStats)
	go func() {
		for {
			select {
			case msg := <-stats:
				statsForAlerts <- msg
				statsForUI <- msg
			case <-ctx.Done():
				return
			}
		}
	}()
	return statsForAlerts, statsForUI
}
