// Helper functions that provide mocks, helpers, test harnesses, etc.
package logmon_test

import (
	"io/ioutil"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus/hooks/test"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

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

	parser := logmon.W3CommonLogParser{}
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

	return f.registry[i], f.raws[i]
}

func flushLogs(t *testing.T, hook *test.Hook) {
	func(hook *test.Hook) {
		t.Log("log dump:")
		entries := hook.AllEntries()
		for _, e := range entries {
			t.Log(e.Message)
		}
		hook.Reset()
	}(hook)
}
