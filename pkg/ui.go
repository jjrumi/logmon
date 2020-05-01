package logmon

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// UIOpts defines the options required to build a UI.
type UIOpts struct {
	Refresh        int
	AlertThreshold int
	AlertWindow    int
}

// NewUI creates a UI.
func NewUI(opts UIOpts) UI {
	return UI{refresh: opts.Refresh, alertThreshold: opts.AlertThreshold, alertWindow: opts.AlertWindow}
}

// UI holds the configuration values of the monitor to display the information.
// It uses a third party library (github.com/gizak/termui) to manipulate the GUI in the console.
type UI struct {
	refresh        int
	alertThreshold int
	alertWindow    int
}

// Setup configures the UI and returns a callback to cleanup afterwards.
func (u UI) Setup() (func(), error) {
	if err := ui.Init(); err != nil {
		return nil, fmt.Errorf("initialize termui: %w", err)
	}

	cleanup := func() {
		log.Printf("clean up: close UI...")
		ui.Close()
	}

	return cleanup, nil
}

// Run builds the layout and loops infinitely consuming traffic stats and alerts.
// It also captures interruption signals.
func (u UI) Run(ctx context.Context, stats <-chan TrafficStats, alertsBus <-chan ThresholdAlert) {
	traffic := u.buildTrafficWidget()
	alerts := u.buildAlertsWidget()
	sections := u.buildSectionsWidget()
	status := u.buildStatusWidget()
	methods := u.buildMethodsWidget()
	config := u.buildConfigWidget()
	grid := u.buildUIGrid(traffic, config, sections, status, methods, alerts)

	ui.Render(grid)
	uiEvents := ui.PollEvents()

LOOP:
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				break LOOP
			}
		case s, ok := <-stats:
			if !ok {
				break LOOP
			}

			config.Rows = u.formatConfig()
			traffic.Rows = u.formatTraffic(s)
			sections.Rows = u.formatSections(s)
			status.Rows = u.formatStatus(s)
			methods.Rows = u.formatMethods(s)

			ui.Render(grid)
		case a, ok := <-alertsBus:
			if !ok {
				break LOOP
			}

			alerts.Rows = u.formatAlerts(a)

			ui.Render(grid)
		case <-ctx.Done():
			break LOOP
		}
	}
}

func (u UI) buildUIGrid(traffic *widgets.List, config interface{}, sections *widgets.List, status interface{}, methods interface{}, alerts *widgets.List) *ui.Grid {
	grid := ui.NewGrid()
	termWidth, termHeight := ui.TerminalDimensions()
	grid.SetRect(0, 0, termWidth, termHeight)

	grid.Set(
		ui.NewRow(0.2,
			ui.NewCol(1.0/2, traffic),
			ui.NewCol(1.0/2, config),
		),
		ui.NewRow(0.2,
			ui.NewCol(1.0, alerts),
		),
		ui.NewRow(0.6,
			ui.NewCol(1.0/2, sections),
			ui.NewCol(1.0/2,
				ui.NewRow(0.5, status),
				ui.NewRow(0.5, methods),
			),
		),
	)
	return grid
}

func (u UI) buildSectionsWidget() *widgets.List {
	sections := widgets.NewList()
	sections.Title = "Top 20 sections"
	sections.WrapText = false
	sections.SetRect(0, 0, 50, 8)
	sections.Rows = []string{
		"",
		"waiting for inputs...",
	}

	return sections
}

func (u UI) buildTrafficWidget() *widgets.List {
	traffic := widgets.NewList()
	traffic.Title = "Traffic"
	traffic.WrapText = false
	traffic.SetRect(0, 0, 50, 8)
	traffic.Rows = []string{
		"",
		"waiting for inputs...",
	}

	return traffic
}

func (u UI) buildAlertsWidget() *widgets.List {
	alerts := widgets.NewList()
	alerts.Title = "Alerts"
	alerts.WrapText = false
	alerts.SetRect(0, 0, 50, 8)
	alerts.Rows = []string{
		"",
		"no alerts triggered",
	}

	return alerts
}

func (u UI) buildStatusWidget() *widgets.List {
	status := widgets.NewList()
	status.Title = "HTTP response status"
	status.WrapText = false
	status.SetRect(0, 0, 50, 8)
	status.Rows = []string{
		"",
		"waiting for inputs...",
	}

	return status
}

func (u UI) buildMethodsWidget() *widgets.List {
	methods := widgets.NewList()
	methods.Title = "HTTP request methods"
	methods.WrapText = false
	methods.SetRect(0, 0, 50, 8)
	methods.Rows = []string{
		"",
		"waiting for inputs...",
	}

	return methods
}

func (u UI) buildConfigWidget() *widgets.List {
	config := widgets.NewList()
	config.Title = "Monitor setup values"
	config.WrapText = false
	config.SetRect(0, 0, 50, 8)
	config.Rows = u.formatConfig()

	return config
}

func (u UI) formatConfig() []string {
	return []string{
		fmt.Sprintf("Current time: %v", time.Now().Format(time.RFC1123)),
		fmt.Sprintf("Refresh interval: [%v](fg:blue)s", u.refresh),
		fmt.Sprintf("Alert threshold: [%v](fg:blue)req/s", u.alertThreshold),
		fmt.Sprintf("Alert window: [%v](fg:blue)s", u.alertWindow),
	}
}

func (u UI) formatTraffic(s TrafficStats) []string {
	return []string{
		"",
		fmt.Sprintf("Total requests: [%v](fg:blue)", s.TotalReqs),
		fmt.Sprintf("Bytes transferred: [%v](fg:blue)", s.Bytes),
	}
}

func (u UI) formatSections(s TrafficStats) []string {
	buf := fromMap(s.SectionHits)

	return buf.marshalTopList("Hits - Section", 20)
}

func (u UI) formatStatus(s TrafficStats) []string {
	buf := fromMap(s.StatusClassHits)

	return buf.marshalTopList("Hits - HTTP status", 10)
}

func (u UI) formatMethods(s TrafficStats) []string {
	buf := fromMap(s.MethodHits)

	return buf.marshalTopList("Hits - HTTP method", 10)
}

func (u UI) formatAlerts(a ThresholdAlert) []string {
	if a.Open {
		return []string{
			"",
			fmt.Sprintf("[!!](fg:red) High traffic generated an alert - hits = [%.2f](fg:red)req/s\n - triggered at %v", a.Hits, a.Time.Format(time.RFC1123)),
		}
	}
	return []string{
		"",
		fmt.Sprintf("[OK](fg:green) High traffic alert recovered - hits = [%.2f](fg:green)req/s\n - recovered at %v", a.Hits, a.Time.Format(time.RFC1123)),
	}
}

// entry is a helper struct to build sorted list of top values from maps
type entry struct {
	val int
	key string
}

type entries []entry

func fromMap(m map[string]int) entries {
	var buf entries
	for k, v := range m {
		buf = append(buf, entry{key: k, val: v})
	}
	return buf
}

func (e entries) Len() int {
	return len(e)
}
func (e entries) Less(i, j int) bool {
	return e[i].val < e[j].val
}
func (e entries) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e entries) marshalTopList(title string, max int) []string {
	if e.Len() < 1 {
		return []string{
			"",
			"waiting for inputs...",
		}
	}

	sort.Sort(sort.Reverse(e))

	output := []string{title}
	count := 0
	for _, v := range e {
		output = append(output, fmt.Sprintf("%v - [%v](fg:blue)", v.val, v.key))
		count++
		if count >= max {
			break
		}
	}
	return output
}
