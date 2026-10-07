// Package main runs the DistroLearn workers service.
package main

import (
	"log/slog"
	"os"
)

func main() {
	slog.Info("workers starting")
	if err := run(); err != nil {
		slog.Error("workers exited with error", "error", err)
		os.Exit(1)
	}
	slog.Info("workers stopped cleanly")
}

// run is the service body. The skeleton does no work yet.
func run() error {
	return nil
}
