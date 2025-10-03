package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRespondWithError(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error { return respondWithError(c, 400, "bad") })

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRespondWithJson(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error { return respondWithJson(c, 201, map[string]string{"ok": "1"}) })

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		t.Fatalf("expected content-type to be set")
	}
}

func TestRespondWithoutBody(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error { return respondWithoutBody(c, 202) })

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != 202 {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
}

func TestRespondWithString(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error { return respondWithString(c, 200, "hello") })

	req := httptest.NewRequest("GET", "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
