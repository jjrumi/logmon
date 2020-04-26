package logmon_test

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/hpcloud/tail"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

var hook *test.Hook
var fixtures LogEntryFixtures

func TestMain(m *testing.M) {
	// Setup:
	var logger *logrus.Logger
	logger, hook = test.NewNullLogger()
	log.SetOutput(logger.Writer())

	fixtures = NewLoadFixtures()

	// Run tests:
	code := m.Run()

	// Tear down:
	os.Exit(code)
}

func TestLogEntryProducer(t *testing.T) {
	producer, file, teardown := givenAW3CommonLogEntryProducer(t)
	defer teardown()

	entries, cleanup, err := producer.Run(context.Background())
	require.NoError(t, err)
	defer cleanup()

	t.Run("it produces one log entry when one line is written to file", func(t *testing.T) {
		entry, raw := fixtures.GetOneAtRandom()
		appendToFile(t, file, raw)

		read := mustRead(t, entries)
		require.EqualValues(t, entry, read)
	})

	t.Run("it produces N log entries when N lines are written to file", func(t *testing.T) {
		for i := 0; i < len(fixtures.raws); i++ {
			appendToFile(t, file, fixtures.raws[i])
		}

		ok := true
		count := 0
		for ok && count < len(fixtures.raws) {
			_, ok = <-entries
			count++
		}

		require.True(t, ok)
		require.Equal(t, len(fixtures.raws), count)
	})
}

func TestLogEntryProducer_Stop(t *testing.T) {
	producer, file, teardown := givenAW3CommonLogEntryProducer(t)
	defer teardown()

	ctx, cancel := context.WithCancel(context.Background())
	entries, cleanup, err := producer.Run(ctx)
	require.NoError(t, err)
	defer cleanup()

	t.Run("it stops tailing on context cancellation", func(t *testing.T) {
		for i := 0; i < len(fixtures.raws); i++ {
			appendToFile(t, file, fixtures.raws[i])
		}

		read := mustRead(t, entries)
		require.False(t, cmp.Equal(logmon.LogEntry{}, read))
		cancel()

		ok := true
		count := 0
		for ok && count < len(fixtures.raws) {
			read, ok = <-entries
			if ok {
				require.False(t, cmp.Equal(logmon.LogEntry{}, read))
				count++
			}
		}

		require.False(t, ok)
		require.True(t, count < len(fixtures.raws))
	})
}

func mustRead(t *testing.T, entries <-chan logmon.LogEntry) (entry logmon.LogEntry) {
	ticker := time.NewTicker(100 * time.Millisecond)
	select {
	case entry = <-entries:
		return entry
	case <-ticker.C:
		t.Fatal("unable to read entry from bus")
	}

	return entry
}

func appendToFile(t *testing.T, file *os.File, content string) {
	b := []byte(content)
	b = append(b, '\n')

	_, err := file.Write(b)
	if err != nil {
		t.Fatal(err)
	}
}

func givenAW3CommonLogEntryProducer(t *testing.T) (logmon.LogEntryProducer, *os.File, func()) {
	// Create a temp log file:
	file, err := ioutil.TempFile("", "logfile_*")
	if err != nil {
		t.Fatalf("create temp log file: %v", err)
	}

	teardown := func() {
		t.Log("tearing down temp file...")
		file.Close()
		os.Remove(file.Name())
	}

	// Create a LogEntryProducer from the temp log file:
	opts := logmon.ProducerOpts{LogFilePath: file.Name(), TailWhence: io.SeekStart, TailLogger: tail.DiscardingLogger}
	producer := logmon.NewLogEntryProducer(opts)

	return producer, file, teardown
}
