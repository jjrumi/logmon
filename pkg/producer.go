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

type LogEntryProducer interface {
	Setup() (func(), error)
	Run(ctx context.Context, entries chan<- LogEntry)
}

func NewLogEntryProducer(opts ProducerOpts) LogEntryProducer {
	tailCfg := tail.Config{
		Follow:    true,
		Location:  &tail.SeekInfo{Offset: 0, Whence: opts.TailWhence},
		ReOpen:    true,
		MustExist: true,
		Logger:    opts.TailLogger,
	}

	return &logEntryProducer{
		filename:   opts.LogFilePath,
		tailCfg:    tailCfg,
		parser:     W3CommonLogParser{},
	}
}

type ProducerOpts struct {
	LogFilePath string
	TailWhence  int // From where start tailing: [io.SeekStart, io.SeekCurrent, io.SeekEnd]
	TailLogger  *log.Logger
}

type logEntryProducer struct {
	filename   string
	tailCfg    tail.Config
	tail       *tail.Tail
	parser     W3CommonLogParser
}

func (p *logEntryProducer) Setup() (func(), error) {
	var err error
	p.tail, err = tail.TailFile(p.filename, p.tailCfg)
	if err != nil {
		return nil, fmt.Errorf("creating log tail: %w", err)
	}

	cleanup := func() {
		log.Printf("cleaning up Producer...\n")
		p.tail.Cleanup()
	}

	return cleanup, nil
}

func (p logEntryProducer) Run(ctx context.Context, entries chan<- LogEntry) {
LOOP:
	for {
		select {
		case line, ok := <-p.tail.Lines:
			if !ok {
				log.Printf("tail channel closed")
				break LOOP
			}

			entry, err := p.parser.Parse(line.Text)
			if err == nil {
				log.Printf("sending: %v\n", entry)
				entries <- entry
			}
		case <-ctx.Done():
			log.Printf("context cancelled\n")
			break LOOP
		}
	}
	log.Printf("closing entries channel\n")
	close(entries)
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
	status, _ = strconv.Atoi(matches[8]) // The regexp ensures it's a string between [000,999].

	var bytes int
	bytes, err = strconv.Atoi(matches[9])
	if err != nil {
		bytes = 0
	}

	return NewLogEntry(
		matches[1],
		matches[2],
		matches[3],
		date,
		matches[5],
		matches[6],
		matches[7],
		status,
		bytes,
	), nil
}
