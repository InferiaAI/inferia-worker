// Package healthz exposes /healthz (liveness) and /readyz (readiness)
// endpoints on the worker's Fiber app.
package healthz

import (
	"sync/atomic"

	"github.com/gofiber/fiber/v2"
)

// Readiness is a thread-safe ready-flag. The worker flips it true after its
// first successful control-channel connection.
type Readiness struct {
	ready atomic.Bool
}

// New creates a Readiness initially set to "not ready".
func New() *Readiness { return &Readiness{} }

// MarkReady transitions to ready. Subsequent calls are no-ops.
func (r *Readiness) MarkReady() { r.ready.Store(true) }

// IsReady reports the current state.
func (r *Readiness) IsReady() bool { return r.ready.Load() }

// Register mounts /healthz and /readyz on app. Both endpoints bypass the
// inference token middleware because they are operational, not application,
// traffic.
func Register(app *fiber.App, r *Readiness) {
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
	app.Get("/readyz", func(c *fiber.Ctx) error {
		if !r.IsReady() {
			return c.Status(fiber.StatusServiceUnavailable).SendString("not ready")
		}
		return c.SendString("ready")
	})
}
