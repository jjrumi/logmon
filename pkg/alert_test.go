package logmon_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

func TestAlertSupervisor_NoAlertsWithTrafficNotExceedingThreshold(t *testing.T) {
	// Fill up stats channel with test data.
	numEntries := 20
	stats := make(chan logmon.TrafficStats, numEntries)
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

func TestAlertSupervisor_AlertsAreRecovered(t *testing.T) {
	// Fill up stats channel with stats that force an alert:
	numEntries := 10
	stats := make(chan logmon.TrafficStats, 2*numEntries)
	sendStats(stats, logmon.TrafficStats{TotalReqs: 11}, numEntries) // Simulate 1.1 req/s
	sendStats(stats, logmon.TrafficStats{TotalReqs: 9}, numEntries)  // Simulate 0.9 req/s
	close(stats)

	// Run alert supervisor:
	threshold := 1 // req/s
	interval := 10 // refresh interval, in seconds
	window := 100  // seconds
	manager := givenAnAlertSupervisor(threshold, interval, window)
	alerts := make(chan logmon.ThresholdAlert, 2)
	manager.Run(context.Background(), stats, alerts)

	// First element is an open alert:
	a, ok := <-alerts
	require.True(t, ok, "alerts channel should be open")
	require.Equal(t, 1.1, a.Hits, "alert for 1.1 req/s expected")
	require.True(t, a.Open, "alert is open")

	// Second element is a recovered alert:
	a, ok = <-alerts
	require.True(t, ok, "alerts channel should be open")
	require.True(t, a.Hits < float64(threshold), "alert recovered when hits are below threshold")
	require.False(t, a.Open, "alert is recovered")
}

func TestAlertSupervisor_ContextCancellationBreaksLoop(t *testing.T) {
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
