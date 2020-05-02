package logmon_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

func TestAlertSupervisor_NoAlertsWithTrafficNotExceedingThreshold(t *testing.T) {
	// Fill up stats channel with test data.
	numWindows := 2
	numEntries := 10
	stats := make(chan logmon.TrafficStats, numWindows*numEntries)
	sendStats(stats, logmon.TrafficStats{TotalReqs: 10}, numEntries) // Simulate 1 req/s
	sendStats(stats, logmon.TrafficStats{TotalReqs: 10}, numEntries) // Simulate 1 req/s
	close(stats)

	// Run alert supervisor:
	threshold := 1 // req/s
	interval := 10 // refresh interval, in seconds
	window := 100  // seconds
	manager := givenAnAlertSupervisor(threshold, interval, window)
	alerts := make(chan logmon.ThresholdAlert)
	manager.Run(context.Background(), stats, alerts)

	// Validate no alerts were produced.
	require.Empty(t, alerts, "no alerts expected")

	a, ok := <-alerts
	require.False(t, ok, "alerts channel should be closed, got:", a)
}

func TestAlertSupervisor_AlertsWithHighTraffic(t *testing.T) {
	// Fill up stats channel with test data.
	numEntries := 10
	stats := make(chan logmon.TrafficStats, numEntries)
	sendStats(stats, logmon.TrafficStats{TotalReqs: 11}, numEntries) // Simulate 1.1 req/s
	close(stats)

	// Run alert supervisor:
	threshold := 1 // req/s
	interval := 10 // refresh interval, in seconds
	window := 100  // seconds
	manager := givenAnAlertSupervisor(threshold, interval, window)
	alerts := make(chan logmon.ThresholdAlert, 1)
	manager.Run(context.Background(), stats, alerts)

	a, ok := <-alerts
	require.True(t, ok, "alerts channel should be open")
	require.Equal(t, 1.1, a.Hits, "alert for 1.1 req/s expected")
	require.True(t, a.Open, "alert is open")
}

// TestAlertSupervisor_AlertsAreRecovered tests alert creation and recovery.
//
// Visual representation of the generated traffic for this test:
// ^
// |           6 6 6 6 6
// | 5 5 5 5 5 | | | | | 5 5 5 5 5
// | | | | | | | | | | | | | | | |
// | | | | | | | | | | | | | | | |
// | | | | | | | | | | | | | | | | 2 2 2 2 2
// | | | | | | | | | | | | | | | | | | | | |
// | | | | | | | | | | | | | | | | | | | | |
// |----------------------------------------->t
//             ^- Alert!         ^- Recover!
func TestAlertSupervisor_AlertsAreRecovered(t *testing.T) {
	// Fill up stats channel with stats that force an alert:
	numWindows := 4
	numEntries := 5
	stats := make(chan logmon.TrafficStats, numWindows*numEntries)
	sendStats(stats, logmon.TrafficStats{TotalReqs: 5}, numEntries) // Simulate 5 req/s - no alert
	sendStats(stats, logmon.TrafficStats{TotalReqs: 6}, numEntries) // Simulate 6 req/s - new alert
	sendStats(stats, logmon.TrafficStats{TotalReqs: 5}, numEntries) // Simulate 5 req/s - alert recovered
	sendStats(stats, logmon.TrafficStats{TotalReqs: 2}, numEntries) // Simulate 2 req/s - no alert
	close(stats)

	// Run alert supervisor:
	threshold := 5 // req/s
	interval := 1  // refresh interval, in seconds
	window := 5    // seconds
	manager := givenAnAlertSupervisor(threshold, interval, window)
	alerts := make(chan logmon.ThresholdAlert, 2)
	manager.Run(context.Background(), stats, alerts)

	// First element is an open alert:
	a, ok := <-alerts
	require.True(t, ok, "alerts channel should be open")
	// 5.2 req/s expected => 4 x 5req + 1 x 6req = 26req over a 5s window ==> 26/5 = 5.2
	require.Equal(t, 5.2, a.Hits, "alert for 1.1 req/s expected")
	require.True(t, a.Open, "alert is open")

	// Second element is a recovered alert:
	a, ok = <-alerts
	require.True(t, ok, "alerts channel should be open")
	// 5.0 req/s expected => 5 x 5req = 25req over a 5s window ==> 25/5 = 5.0
	require.Equal(t, 5.0, a.Hits, "alert recovered when hits are below threshold")
	require.False(t, a.Open, "alert is recovered")

	// No new alerts were triggered as following traffic was below threshold.
	require.Empty(t, alerts, "no alerts expected")

	a, ok = <-alerts
	require.False(t, ok, "alerts channel should be closed, got:", a)
}

func TestAlertSupervisor_ContextCancellationEndsTheSupervisor(t *testing.T) {
	// Run alert supervisor:
	manager := givenAnAlertSupervisor(1, 10, 100)
	stats := make(chan logmon.TrafficStats)
	alerts := make(chan logmon.ThresholdAlert)
	ctx, cancel := context.WithCancel(context.Background())
	go manager.Run(ctx, stats, alerts)

	// Force a context cancellation:
	cancel()

	_, ok := <-alerts
	require.False(t, ok, "alerts channel should be closed")
}

func sendStats(stats chan logmon.TrafficStats, trafficStats logmon.TrafficStats, entries int) int {
	expectedHits := 0
	for i := 0; i < entries; i++ {
		stats <- trafficStats
		expectedHits += trafficStats.TotalReqs
	}

	return expectedHits
}

func givenAnAlertSupervisor(threshold int, interval int, window int) logmon.AlertSupervisor {
	return logmon.NewAlertsSupervisor(
		logmon.AlertSupervisorOpts{
			AlertThreshold:  threshold,
			RefreshInterval: interval,
			AlertWindow:     window,
		})
}
