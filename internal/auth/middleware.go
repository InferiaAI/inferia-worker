package auth

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// NewInferenceTokenMiddleware returns a Fiber handler that requires
//
//	Authorization: Bearer <expected>
//
// on every request whose path starts with /v1/. Other paths (healthz, readyz,
// metrics) are passed through. Token comparison is constant-time to avoid
// leaking timing information.
func NewInferenceTokenMiddleware(expected string) fiber.Handler {
	expectedBytes := []byte(expected)
	return func(c *fiber.Ctx) error {
		if !strings.HasPrefix(c.Path(), "/v1/") {
			return c.Next()
		}
		raw := c.Get("Authorization")
		got := extractBearer(raw)
		if got == "" {
			return c.Status(fiber.StatusUnauthorized).SendString("missing or malformed Authorization header")
		}
		if !constantTimeEqual([]byte(got), expectedBytes) {
			return c.Status(fiber.StatusUnauthorized).SendString("invalid inference token")
		}
		return c.Next()
	}
}

// extractBearer returns the token following a single-space "Bearer " prefix.
// Anything malformed (wrong scheme, missing token, extra whitespace, extra
// tokens) returns "". RFC 6750 specifies a single SP separator and a single
// b64token component.
func extractBearer(h string) string {
	if h == "" {
		return ""
	}
	// Header is "<scheme> <token>" — split on the first space.
	sp := strings.IndexByte(h, ' ')
	if sp <= 0 {
		return ""
	}
	scheme := h[:sp]
	rest := h[sp+1:]
	if !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	// Reject leading whitespace (double-space, tab, …) and multiple tokens.
	if rest == "" {
		return ""
	}
	if rest[0] == ' ' || rest[0] == '\t' {
		return ""
	}
	if strings.ContainsAny(rest, " \t") {
		return ""
	}
	return rest
}

func constantTimeEqual(a, b []byte) bool {
	if len(a) != len(b) {
		// subtle.ConstantTimeCompare also returns 0 for length mismatch, but
		// short-circuiting here is explicit and avoids confusion in code review.
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
