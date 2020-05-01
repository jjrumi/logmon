package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	logmon "github.com/jjrumi/accesslogmonitor/pkg"
)

var (
	// Command line flags.
	logFilePath     string
	refreshInterval int
	alertThreshold  int
	alertWindow     int
)

// setLogger uses a file to log while on "debug" mode. No logging otherwise.
func setLogger() *os.File {
	level, ok := os.LookupEnv("LOG_LEVEL")
	if !ok || level != "debug" {
		log.SetOutput(ioutil.Discard)
		return nil
	}

	// Setup log for debugging:
	f, err := os.OpenFile("/tmp/logmon.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		log.Fatal(err)
	}

	logger := log.New(f, "", log.LstdFlags)
	log.SetOutput(logger.Writer())
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile | log.LUTC)
	return f
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
	logFile := setLogger()
	if logFile != nil {
		defer logFile.Close()
	}

	setArguments()
	flag.Parse()

	opts := logmon.MonitorOpts{
		LogFilePath:     logFilePath,
		RefreshInterval: refreshInterval,
		AlertThreshold:  alertThreshold,
		AlertWindow:     alertWindow,
	}
	monitor := logmon.NewMonitor(opts)

	// UI loops until an interrupt signal is captured.
	err := monitor.Run(context.Background())
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
