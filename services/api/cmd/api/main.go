// Package main runs the DistroLearn API service.
package main

import (
	"log/slog"
	"os"
)

func main() {
	slog.Info("api starting")
	if err := run(); err != nil {
		slog.Error("api exited with error", "error", err)
		os.Exit(1)
	}
	slog.Info("api stopped cleanly")
}

// run is the service body. The skeleton does no work yet.
func run() error {
	return nil
}
