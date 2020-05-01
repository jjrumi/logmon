package logmon_test

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/nxadm/tail"
	"github.com/stretchr/testify/require"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

var fixtures LogEntryFixtures

func TestMain(m *testing.M) {
	// Setup:
	log.SetOutput(ioutil.Discard)
	fixtures = NewLoadFixtures()

	// Run tests:
	code := m.Run()

	// Tear down:
	os.Exit(code)
}

func TestLogEntryProducer_WithInvalidFile(t *testing.T) {
	opts := logmon.ProducerOpts{LogFilePath: "invalid-file-path"}
	producer := logmon.NewLogEntryProducer(opts)
	_, err := producer.Setup()
	require.Error(t, err)
}

func TestLogEntryProducer_ForNLines(t *testing.T) {
	// Setup producer and file to write to:
	file, entries, cancel, teardown := setupFileAndProducer(t)
	defer cancel()
	defer teardown()

	// Write entries to log:
	for i := 0; i < len(fixtures.raws); i++ {
		go appendToFile(file, fixtures.raws[i])
	}

	// Read all the entries from the channel:
	ok := true
	count := 0
	var read logmon.LogEntry
	for ok && count < len(fixtures.raws) {
		read, ok = <-entries
		if ok {
			require.False(t, equalLogEntries(read, givenAnEmptyLogEntry()), "entry is not empty")
		}
		count++
	}

	require.True(t, ok, "all reads were successful")
	require.Equal(t, len(fixtures.raws), count, "all lines have been read")
}

func TestLogEntryProducer_ContextCancellation(t *testing.T) {
	// Setup producer and file to write to:
	file, entries, cancel, teardown := setupFileAndProducer(t)
	defer teardown()

	// Write entries to log:
	for i := 0; i < len(fixtures.raws); i++ {
		go appendToFile(file, fixtures.raws[i])
	}

	// Read entries while channel is open:
	ok := true
	count := 0
	var read logmon.LogEntry
	for ok && count < len(fixtures.raws) {
		read, ok = <-entries
		if ok {
			require.False(t, equalLogEntries(read, givenAnEmptyLogEntry()), "entry is not empty")
			count++
		}

		// Force producer context cancellation:
		if count == 3 {
			cancel()
		}
	}

	require.False(t, ok, "the producer closed the channel")
	require.True(t, count < len(fixtures.raws), "not all the written lines were read")
}

func setupFileAndProducer(t *testing.T) (*os.File, <-chan logmon.LogEntry, context.CancelFunc, func()) {
	// Create a temp log file:
	file, err := ioutil.TempFile("", "logfile_*")
	require.NoError(t, err)

	// Create producer that will feed from the temp file:
	ctx, cancel := context.WithCancel(context.Background())
	producer := givenALogEntryProducer(file)
	cleanup, err := producer.Setup()
	require.NoError(t, err)

	// Run producer in separate goroutine:
	entries := make(chan logmon.LogEntry)
	go producer.Run(ctx, entries)

	teardown := func() {
		cleanup()

		file.Close()
		os.Remove(file.Name())
	}
	return file, entries, cancel, teardown
}

func givenALogEntryProducer(file *os.File) logmon.LogEntryProducer {
	opts := logmon.ProducerOpts{
		LogFilePath: file.Name(),
		TailWhence: io.SeekStart,
		TailLogger: tail.DiscardingLogger,
		LogParser: logmon.NewW3CommonLogParser(),
	}
	producer := logmon.NewLogEntryProducer(opts)

	return producer
}

func TestW3CommonLogParser(t *testing.T) {
	parser := logmon.NewW3CommonLogParser()
	entryA, rawA := fixtures.GetOneAtRandom()
	entryB, rawB := fixtures.GetOneAtRandom()

	// Manually replace bytes transferred value with a dash:
	entryB.Bytes = 0
	rawB = rawB[:strings.LastIndex(rawB, " ")+1] + "-"

	for name, tc := range map[string]struct {
		rawLogEntry   string
		expectedEntry logmon.LogEntry

		succeeds bool
	}{
		"it parses valid log lines": {
			rawLogEntry:   rawA,
			expectedEntry: entryA,
			succeeds:      true,
		},
		"it fails when parsing an invalid log line": {
			rawLogEntry:   `invalid-log-entry`,
			expectedEntry: givenAnEmptyLogEntry(),
			succeeds:      false,
		},
		"it fails when parsing an invalid date value": {
			rawLogEntry:   `72.157.153.74 - - [xxxx] "PUT /seamless/whiteboard/holistic/mesh HTTP/2.0" 204 14813`,
			expectedEntry: givenAnEmptyLogEntry(),
			succeeds:      false,
		},
		"it defaults to zero for entries with bytes represented with a dash '-'": {
			rawLogEntry:   rawB,
			expectedEntry: entryB,
			succeeds:      true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			read, err := parser.Parse(tc.rawLogEntry)
			require.Equal(t, tc.succeeds, err == nil)
			require.True(t, equalLogEntries(read, tc.expectedEntry))
		})
	}
}
