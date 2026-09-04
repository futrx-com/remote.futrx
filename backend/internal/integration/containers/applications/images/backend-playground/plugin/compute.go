package main

import (
	"net/http"
	"runtime"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// compute is the reason to have a Go backend at all: work that would be wrong
// to do in a browser. The numbers are deliberately boring; the point is that
// they were produced by a compiled process on the server.
func (p *playground) compute(request appplugin.Request) appplugin.Response {
	var body struct {
		N int `json:"n"`
	}
	if err := request.DecodeJSON(&body); err != nil {
		return appplugin.Errorf(http.StatusBadRequest, "invalid json: %v", err)
	}
	if body.N < 0 || body.N > 90 {
		return appplugin.Errorf(http.StatusBadRequest, "n must be between 0 and 90")
	}
	started := time.Now()
	previous, current := uint64(0), uint64(1)
	for i := 0; i < body.N; i++ {
		previous, current = current, previous+current
	}
	return appplugin.JSON(http.StatusOK, map[string]any{
		"n":           body.N,
		"fibonacci":   previous,
		"primes":      countPrimes(200000),
		"elapsedNs":   time.Since(started).Nanoseconds(),
		"computedBy":  runtime.Version(),
		"onGoroutine": runtime.NumGoroutine(),
	})
}

func countPrimes(limit int) int {
	composite := make([]bool, limit+1)
	count := 0
	for candidate := 2; candidate <= limit; candidate++ {
		if composite[candidate] {
			continue
		}
		count++
		for multiple := candidate * candidate; multiple <= limit; multiple += candidate {
			composite[multiple] = true
		}
	}
	return count
}
