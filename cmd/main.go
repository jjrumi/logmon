package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

var (
	// Command line flags.
	logFilePath     string
	refreshInterval int
	alertThreshold  int
	alertWindow     time.Duration
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
	flag.DurationVar(&alertWindow, "window", 2*time.Minute, "time period to check the alert condition, in minutes")

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
		RefreshInterval: refreshInterval * 1000, // Store refresh interval in ms.
		AlertThreshold:  alertThreshold,
		AlertWindow:     alertWindow,
	}

	monitor := logmon.NewMonitor(opts)

	ctx, cancel := context.WithCancel(context.Background())
	cleanup, err := monitor.Run(ctx)
	if err != nil {
		// TODO: log.Fatal do not log errors... it logs infos.
		log.Fatal(err)
	}

	// Listen for an interrupt or terminate signal from the OS.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	cancel()
	cleanup()
	os.Exit(0)
}
