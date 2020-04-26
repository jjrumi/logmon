package logmon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	"github.com/hpcloud/tail"
)

type LogEntryProducer interface {
	Run(ctx context.Context) (<-chan LogEntry, func(), error)
}

func NewLogEntryProducer(opts ProducerOpts) LogEntryProducer {
	tailCfg := tail.Config{
		Follow:   true,
		Location: &tail.SeekInfo{Offset: 0, Whence: opts.TailWhence},
		ReOpen:   true,
		Logger:   opts.TailLogger,
	}

	return logEntryProducer{
		filename: opts.LogFilePath,
		tailCfg:  tailCfg,
		parser:   W3CommonLogParser{},
	}
}

type ProducerOpts struct {
	LogFilePath string
	TailWhence  int // From where start tailing: [io.SeekStart, io.SeekCurrent, io.SeekEnd]
	TailLogger  *log.Logger
}

type logEntryProducer struct {
	filename string
	tailCfg  tail.Config
	parser   W3CommonLogParser
}

func (p logEntryProducer) Run(ctx context.Context) (<-chan LogEntry, func(), error) {
	watcher, err := tail.TailFile(p.filename, p.tailCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("creating log tail: %w", err)
	}

	entries := make(chan LogEntry)

	go func() {
	LOOP:
		for {
			select {
			case line, ok := <-watcher.Lines:
				if !ok {
					log.Printf("tail channel closed")
					break LOOP
				}
				log.Printf("new line: %v", line.Text)

				entry, err := p.parser.Parse(line.Text)
				if err != nil {
					// TODO: Inject logger
					log.Printf("err parsing: %v", err)
					continue
				}
				entries <- entry
			case <-ctx.Done():
				break LOOP
			}
		}
		log.Printf("closing entries channel\n")
		close(entries)
	}()

	cleanup := func() {
		log.Printf("cleaning up Producer...\n")
		watcher.Cleanup()
	}

	return entries, cleanup, nil
}

type W3CommonLogParser struct{}

// Parse uses regexp to capture groups in a log file the following format:
// https://www.w3.org/Daemon/User/Config/Logging.html#common-logfile-format
// example input:
//   145.22.59.60 - - [24/Apr/2020:18:10:14 +0000] "PUT /web-enabled/enterprise/dynamic HTTP/1.0" 200 22035
func (p W3CommonLogParser) Parse(line string) (entry LogEntry, err error) {
	rx := regexp.MustCompile(
		// Capture groups in: remotehost rfc931 authuser [date] "request" status bytes
		`^(\S+) (\S+) (\S+) \[([^]]+)] "(\S+) ([^"]+) (\S+)" ([0-9]{3}) ([0-9]+|-)$`,
	)

	matches := rx.FindStringSubmatch(line)
	if len(matches) < 9 {
		return entry, errors.New("parse log entry")
	}

	var date time.Time
	date, err = time.Parse("02/Jan/2006:15:04:05 -0700", matches[4])
	if err != nil {
		return entry, fmt.Errorf("parse date from log: %w", err)
	}

	var status int
	status, err = strconv.Atoi(matches[8])
	if err != nil {
		return entry, fmt.Errorf("parse status from log: %w", err)
	}

	var bytes int
	bytes, err = strconv.Atoi(matches[9])
	if err != nil {
		bytes = 0
	}

	return LogEntry{
		RemoteHost:  matches[1],
		UserID:      matches[2],
		Username:    matches[3],
		Date:        date,
		ReqMethod:   matches[5],
		ReqPath:     matches[6],
		ReqProtocol: matches[7],
		StatusCode:  status,
		Bytes:       bytes,
	}, nil
}
