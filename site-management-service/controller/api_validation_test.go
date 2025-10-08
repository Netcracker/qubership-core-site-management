package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// These tests exercise validation/error branches that do not require Synchronizer

func TestGetSite_MissingURLHeader(t *testing.T) {
	h := &ApiHttpHandler{Synchronizer: nil}
	app := fiber.New()
	app.Get("/api/v1/routes/:tenantId/site", h.GetSite)

	req := httptest.NewRequest("GET", "/api/v1/routes/tenant-1/site", nil)
	// missing URL header triggers 400 before synchronizer usage
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetServiceName_MissingTenantHeader(t *testing.T) {
	h := &ApiHttpHandler{}
	app := fiber.New()
	app.Get("/api/v1/tenants/current/service/name", h.GetServiceName)

	req := httptest.NewRequest("GET", "/api/v1/tenants/current/service/name", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestValidate_BadJson(t *testing.T) {
	h := &ApiHttpHandler{}
	app := fiber.New()
	app.Post("/api/v1/validate", h.Validate)

	req := httptest.NewRequest("POST", "/api/v1/validate", strings.NewReader("{bad}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
