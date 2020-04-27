package logmon

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"
)

type MonitorOpts struct {
	LogFilePath     string
	RefreshInterval int
	AlertThreshold  int
	AlertWindow     time.Duration
}

type Monitor struct {
	producer   LogEntryProducer
	supervisor TrafficSupervisor
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

type AlertsManager struct {
}

type UI struct {
}

func NewMonitor(opts MonitorOpts) *Monitor {
	producerOpts := ProducerOpts{opts.LogFilePath, io.SeekEnd, nil}
	producer := NewLogEntryProducer(producerOpts)

	supervisorOpts := SupervisorOpts{opts.RefreshInterval}
	supervisor := NewTrafficSupervisor(supervisorOpts)

	// TODO: Create an alerts manager.

	// TODO: Create a UI.

	// TODO: Yield flow control to UI ??

	return &Monitor{producer: producer, supervisor: supervisor}
}

func (m Monitor) Run(ctx context.Context) (func(), error) {
	entries, cleanupProducer, err := m.producer.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting log entry producer: %w", err)
	}
	stats := m.supervisor.Run(ctx, entries)

	/*
		alerts := m.alerts.Run(ctx, statsForAlerts)
		m.ui.Run(ctx, statsForUI, alerts)


	*/

	go func() {
	LOOP:
		for {
			select {
			case entry, ok := <-stats:
				if !ok {
					log.Printf("stats channel closed")
					break LOOP
				}
				// TODO: Move fmt.Printx to the UI
				fmt.Printf("%v\n", entry)
				// TODO:
				//  - Broadcast stats
				//  - Send stats to both Alerts Manager & UI
				/*
					for {
						msg := <-stats
						statsForAlerts <- msg
						statsForUI <- msg
					}
				*/
			case <-ctx.Done():
				break LOOP
			}
		}
	}()

	return func() {
		cleanupProducer()

		// m.alertsMng.Cleanup()
		// m.ui.Cleanup()
	}, nil
}
