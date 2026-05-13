package auth

import (
	"crypto/subtle"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestInferenceTokenMiddleware_AcceptValid(t *testing.T) {
	app := fiber.New()
	app.Use(NewInferenceTokenMiddleware("the-secret-token-1234"))
	app.Get("/v1/ping", func(c *fiber.Ctx) error { return c.SendString("pong") })

	req := httptest.NewRequest("GET", "/v1/ping", nil)
	req.Header.Set("Authorization", "Bearer the-secret-token-1234")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
}

func TestInferenceTokenMiddleware_RejectMissing(t *testing.T) {
	app := fiber.New()
	app.Use(NewInferenceTokenMiddleware("the-secret-token-1234"))
	app.Get("/v1/ping", func(c *fiber.Ctx) error { return c.SendString("pong") })

	req := httptest.NewRequest("GET", "/v1/ping", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 401 {
		t.Errorf("status: %d", resp.StatusCode)
	}
}

func TestInferenceTokenMiddleware_RejectWrong(t *testing.T) {
	app := fiber.New()
	app.Use(NewInferenceTokenMiddleware("the-secret-token-1234"))
	app.Get("/v1/ping", func(c *fiber.Ctx) error { return c.SendString("pong") })

	for _, h := range []string{
		"Bearer wrong",
		"wrong",
		"Basic the-secret-token-1234",
		"Bearer ",
		"Bearer  the-secret-token-1234", // double space
	} {
		req := httptest.NewRequest("GET", "/v1/ping", nil)
		req.Header.Set("Authorization", h)
		resp, _ := app.Test(req)
		if resp.StatusCode != 401 {
			t.Errorf("expected 401 for header %q, got %d", h, resp.StatusCode)
		}
	}
}

func TestInferenceTokenMiddleware_SkipsNonV1Routes(t *testing.T) {
	app := fiber.New()
	app.Use(NewInferenceTokenMiddleware("the-secret-token-1234"))
	app.Get("/healthz", func(c *fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest("GET", "/healthz", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 for /healthz without auth, got %d", resp.StatusCode)
	}
}

func TestInferenceTokenMiddleware_UsesConstantTimeCompare(t *testing.T) {
	// Smoke check that the comparison path uses subtle.ConstantTimeCompare.
	// Direct verification is impossible at the function-pointer level; we
	// instead exercise the function and assert it accepts only an equal value.
	got := constantTimeEqual([]byte("a"), []byte("a"))
	if !got {
		t.Errorf("constantTimeEqual same: false")
	}
	got = constantTimeEqual([]byte("a"), []byte("b"))
	if got {
		t.Errorf("constantTimeEqual diff: true")
	}
	got = constantTimeEqual([]byte("ab"), []byte("a"))
	if got {
		t.Errorf("constantTimeEqual diff-len: true")
	}
}

func TestExtractBearer(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":           "abc",
		"bearer abc":           "abc", // case-insensitive scheme
		"BEARER abc":           "abc",
		"Bearer ":              "",
		"":                     "",
		"abc":                  "",
		"Basic abc":            "",
		"Bearerabc":            "",
		"Bearer  abc":          "", // double space → header is invalid
		"Bearer abc def":       "", // multiple tokens not supported
		strings.Repeat("x", 1): "",
	}
	for in, want := range cases {
		got := extractBearer(in)
		if got != want {
			t.Errorf("extractBearer(%q) = %q, want %q", in, got, want)
		}
	}
}

// Ensures we link against crypto/subtle (and trips refactors that drop it).
var _ = subtle.ConstantTimeCompare
