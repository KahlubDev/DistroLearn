// Package healthcheck is shared by the API and workers so both binaries can act as their
// own container probe.
//
// The runtime images are distroless and carry no shell, so a CMD-SHELL healthcheck cannot
// run. The service binary already exists inside the image, so asking it to probe its own
// endpoint is the cheapest option that works.
package healthcheck

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Run returns 0 when the URL answers 2xx, 1 otherwise. The URL is printed on failure so a
// `docker inspect` of an unhealthy container shows why.
func Run(url string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Println("healthcheck: build request:", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("healthcheck:", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		fmt.Printf("healthcheck: %s returned %d\n", url, resp.StatusCode)
		return 1
	}
	return 0
}
