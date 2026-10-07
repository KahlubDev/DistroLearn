package main

import "testing"

func TestRunReturnsNoError(t *testing.T) {
	t.Parallel()

	if err := run(); err != nil {
		t.Fatalf("run() = %v, want nil", err)
	}
}
