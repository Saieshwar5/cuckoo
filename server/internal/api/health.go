package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
)

// HealthCheck reports whether one dependency is usable.
type HealthCheck func(ctx context.Context) error

// healthTimeout bounds the whole check. A health endpoint that can hang is
// worse than one that fails, because a load balancer waits on it.
const healthTimeout = 2 * time.Second

type healthResponse struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components"`
}

// healthHandler reports the server and its dependencies.
//
// It returns 503 when any dependency is down, so a process that is running but
// cannot reach Postgres is correctly treated as not ready to serve traffic.
// Failure reasons are named per component but never include the underlying
// error, which can carry connection strings.
func healthHandler(checks map[string]HealthCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
		defer cancel()

		components := make(map[string]string, len(checks))
		healthy := true

		for name, check := range checks {
			if err := check(ctx); err != nil {
				components[name] = "down"
				healthy = false
				continue
			}
			components[name] = "ok"
		}

		status := http.StatusOK
		overall := "ok"
		if !healthy {
			status = http.StatusServiceUnavailable
			overall = "degraded"
		}

		httpx.JSON(w, r, status, healthResponse{Status: overall, Components: components})
	}
}
