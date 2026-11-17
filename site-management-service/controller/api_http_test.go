package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"
	fiberserver "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3"
	fiberSec "github.com/netcracker/qubership-core-lib-go-fiber-server-utils/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseHelpers(t *testing.T) {
	app := newHttpApp(t)
	app.Get("/json", func(c fiber.Ctx) error {
		return respondWithJson(c, http.StatusOK, map[string]string{"name": "value"})
	})
	app.Get("/error", func(c fiber.Ctx) error {
		return respondWithError(c, http.StatusBadRequest, "something failed")
	})
	app.Get("/no-body", func(c fiber.Ctx) error {
		return respondWithoutBody(c, http.StatusCreated)
	})
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "respondWithJson", path: "/json", wantStatus: http.StatusOK, wantBody: `{"name":"value"}`},
		{name: "respondWithError", path: "/error", wantStatus: http.StatusBadRequest, wantBody: `{"error":"something failed"}`},
		{name: "respondWithoutBody", path: "/no-body", wantStatus: http.StatusCreated, wantBody: `""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, contentType, body := send(t, app, httptest.NewRequest(http.MethodGet, tt.path, nil))

			assert.Equal(t, tt.wantStatus, status, "status")
			assert.Equal(t, "application/json", contentType, "Content-Type")
			assert.Equal(t, tt.wantBody, body, "body")
		})
	}
}

func TestRegisterTenantRejectsUnparsableBody(t *testing.T) {
	app := newHttpApp(t)
	app.Post("/api/v1/tenants", (&ApiHttpHandler{}).RegisterTenant)
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "malformed JSON", contentType: "application/json", body: `{"objectId":`},
		{name: "no Content-Type", body: `{"objectId":"tenant"}`},
		{name: "text/plain", contentType: "text/plain", body: `{"objectId":"tenant"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			status, contentType, body := send(t, app, req)

			assert.Equal(t, http.StatusBadRequest, status, "status")
			assert.Equal(t, "application/json", contentType, "Content-Type")
			assert.Equal(t, `{"error":"Invalid request payload"}`, body, "body")
		})
	}
}

var registerSecurityOnce sync.Once

func newHttpApp(t *testing.T) *fiber.App {
	t.Helper()
	registerSecurityOnce.Do(func() {
		serviceloader.Register(1, &fiberSec.DummyFiberServerSecurityMiddleware{})
	})
	configloader.Init(configloader.EnvPropertySource())
	app, err := fiberserver.New().Process()
	require.NoError(t, err)
	return app
}

func send(t *testing.T, app *fiber.App, req *http.Request) (int, string, string) {
	t.Helper()
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err, "%s %s", req.Method, req.RequestURI)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(body)
}
