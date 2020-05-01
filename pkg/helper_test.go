// Helper functions that provide mocks, helpers, test harnesses, etc.
package logmon_test

import (
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

// LogEntryFixtures contains random log entries to facilitate testing.
type LogEntryFixtures struct {
	registry []logmon.LogEntry
	raws     []string
}

func NewLoadFixtures() LogEntryFixtures {
	f := LogEntryFixtures{}
	f.loadFixtures()
	return f
}

func (f *LogEntryFixtures) loadFixtures() {
	content, _ := ioutil.ReadFile("testdata/100entries.log")

	parser := logmon.NewW3CommonLogParser()
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		entry, _ := parser.Parse(line)
		f.registry = append(f.registry, entry)
		f.raws = append(f.raws, line)
	}
}

func (f *LogEntryFixtures) GetOneAtRandom() (logmon.LogEntry, string) {
	rand.Seed(time.Now().UnixNano())
	min := 0
	max := len(f.registry) - 1
	i := rand.Intn(max-min) + min

	// Fix the creation time on each call to this method:
	entry := f.registry[i]
	entry.CreatedAt = time.Now()

	return entry, f.raws[i]
}

func appendToFile(file *os.File, content string) {
	b := []byte(content)
	b = append(b, '\n')
	_, _ = file.Write(b)
}

func equalTrafficStats(a logmon.TrafficStats, b logmon.TrafficStats) bool {
	return cmp.Equal(a, b)
}

func equalLogEntries(a logmon.LogEntry, b logmon.LogEntry) bool {
	return cmp.Equal(
		a,
		b,
		cmpopts.IgnoreFields(logmon.LogEntry{}, "CreatedAt"),
	)
}

// givenAnEmptyLogEntry creates an empty LogEntry.
func givenAnEmptyLogEntry() logmon.LogEntry {
	return logmon.LogEntry{CreatedAt: time.Now()}
}
