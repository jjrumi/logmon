package logmon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	"github.com/nxadm/tail"
)

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

// NewLogEntry creates a filled LogEntry.
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

// LogEntryProducer watches a log file and produces a LogEntry for each new line.
type LogEntryProducer interface {
	Setup() (func(), error)
	Run(ctx context.Context, entries chan<- LogEntry)
}

// ProducerOpts defines the options required to build a LogEntryProducer.
type ProducerOpts struct {
	LogFilePath string
	TailWhence  int // From where start tailing: [io.SeekStart, io.SeekCurrent, io.SeekEnd]
	TailLogger  *log.Logger
	LogParser   LogParser
}

// logEntryProducer implements the LogEntryProducer interface.
type logEntryProducer struct {
	filename string
	tailCfg  tail.Config
	tail     *tail.Tail
	parser   LogParser
}

// NewLogEntryProducer creates a LogEntryProducer.
func NewLogEntryProducer(opts ProducerOpts) LogEntryProducer {
	tailCfg := tail.Config{
		Follow:    true,
		Location:  &tail.SeekInfo{Offset: 0, Whence: opts.TailWhence},
		ReOpen:    true,
		MustExist: true,
		Logger:    opts.TailLogger,
	}

	return &logEntryProducer{
		filename: opts.LogFilePath,
		tailCfg:  tailCfg,
		parser:   opts.LogParser,
	}
}

// Setup prepares the file watcher on the log file.
// It returns a callback to do a cleanup on the file watcher.
func (p *logEntryProducer) Setup() (func(), error) {
	var err error
	p.tail, err = tail.TailFile(p.filename, p.tailCfg)
	if err != nil {
		return nil, fmt.Errorf("create log tail: %w", err)
	}

	cleanup := func() {
		log.Printf("clean up: remove log tail...")
		p.tail.Cleanup()
	}

	return cleanup, nil
}

// Run consumes new lines from the file watcher and produces LogEntry into an output channel.
func (p logEntryProducer) Run(ctx context.Context, entries chan<- LogEntry) {
LOOP:
	for {
		select {
		case line, ok := <-p.tail.Lines:
			if !ok {
				break LOOP
			}

			entry, err := p.parser.Parse(line.Text)
			if err != nil {
				log.Printf("error parsing log line: %v", err)
				continue
			}

			log.Printf("send log entry: %v", entry)
			entries <- entry
		case <-ctx.Done():
			break LOOP
		}
	}
	log.Printf("clean up: close entries channel")
	close(entries)
}

// LogParser defines a log parser that produces a LogEntry from a log line.
type LogParser interface {
	Parse(line string) (entry LogEntry, err error)
}

// w3CommonLogParser implements the LogParser interface.
type w3CommonLogParser struct{}

func NewW3CommonLogParser() LogParser {
	return w3CommonLogParser{}
}

// Parse uses regexp to capture groups in a log file the following format:
// https://www.w3.org/Daemon/User/Config/Logging.html#common-logfile-format
// example input:
//   145.22.59.60 - - [24/Apr/2020:18:10:14 +0000] "PUT /web-enabled/enterprise/dynamic HTTP/1.0" 200 22035
func (p w3CommonLogParser) Parse(line string) (entry LogEntry, err error) {
	rx := regexp.MustCompile(
		// Capture groups in: remotehost rfc931 authuser [date] "request" status bytes
		`^(\S+) (\S+) (\S+) \[([^]]+)] "(\S+) ([^"]+) (\S+)" ([0-9]{3}) ([0-9]+|-)$`,
	)

	matches := rx.FindStringSubmatch(line)
	if len(matches) < 9 {
		return entry, errors.New("log entry does not match regexp")
	}

	var date time.Time
	date, err = time.Parse("02/Jan/2006:15:04:05 -0700", matches[4])
	if err != nil {
		return entry, fmt.Errorf("parse date from log: %w", err)
	}

	var status int
	status, _ = strconv.Atoi(matches[8]) // The regexp ensures it's a string between [000,999].

	var bytes int
	bytes, err = strconv.Atoi(matches[9])
	if err != nil {
		bytes = 0
	}

	return NewLogEntry(
		matches[1], // host
		matches[2], // userID
		matches[3], // userName
		date,
		matches[5], // http method
		matches[6], // url path
		matches[7], // http protocol
		status,
		bytes,
	), nil
}
