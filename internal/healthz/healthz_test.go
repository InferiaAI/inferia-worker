package healthz

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestLiveness(t *testing.T) {
	r := New()
	app := fiber.New()
	Register(app, r)
	req := httptest.NewRequest("GET", "/healthz", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("status: %d", resp.StatusCode)
	}
}

func TestReadiness_NotReadyBeforeMark(t *testing.T) {
	r := New()
	app := fiber.New()
	Register(app, r)
	req := httptest.NewRequest("GET", "/readyz", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 503 {
		t.Errorf("expected 503 before mark, got %d", resp.StatusCode)
	}
}

func TestReadiness_AfterMark(t *testing.T) {
	r := New()
	r.MarkReady()
	app := fiber.New()
	Register(app, r)
	req := httptest.NewRequest("GET", "/readyz", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after mark, got %d", resp.StatusCode)
	}
}

func TestReadiness_Concurrent(t *testing.T) {
	r := New()
	for i := 0; i < 100; i++ {
		go r.MarkReady()
		go func() { _ = r.IsReady() }()
	}
}
