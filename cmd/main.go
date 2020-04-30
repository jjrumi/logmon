package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/sirupsen/logrus"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

var (
	// Command line flags.
	logFilePath     string
	refreshInterval int
	alertThreshold  int
	alertWindow     int
)

func init() {
	setLogger()
	setArguments()
}

func setLogger() {
	level, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		level = "error"
	}
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.ErrorLevel
	}

	logger := logrus.New()
	logger.SetLevel(lvl)
	log.SetOutput(logger.Writer())
}

func setArguments() {
	flag.StringVar(&logFilePath, "source", "/tmp/access.log", "log file path to monitor")
	flag.IntVar(&refreshInterval, "refresh", 10, "refresh interval at which traffic stats are computed, in seconds")
	flag.IntVar(&alertThreshold, "threshold", 10, "alert condition, in requests per second")
	flag.IntVar(&alertWindow, "window", 120, "time period to check the alert condition, in seconds")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "OPTIONS:")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	opts := logmon.MonitorOpts{
		LogFilePath:     logFilePath,
		RefreshInterval: refreshInterval,
		AlertThreshold:  alertThreshold,
		AlertWindow:     alertWindow,
	}
	monitor := logmon.NewMonitor(opts)

	ctx, cancel := context.WithCancel(context.Background())
	err := monitor.Run(ctx)
	if err != nil {
		// TODO: log.Fatal do not log errors... it logs infos.
		log.Fatal(err)
	}

	cancel()
	os.Exit(0)
}
