package controller

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gofiber/fiber/v2"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
	mock_controller "github.com/netcracker/qubership-core-site-management/site-management-service/v2/controller/mock"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/dao/pg"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/idp"
	pmClientDomain "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/synchronizer"
	mock_synchronizer "github.com/netcracker/qubership-core-site-management/site-management-service/v2/synchronizer/mock"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/tm"
	"github.com/stretchr/testify/assert"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"go.uber.org/mock/gomock"
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

func setupTestHandlers(ctrl *gomock.Controller) (*fiber.App, *mock_controller.MockSynchronizer, *ApiHttpHandler) {
	app := fiber.New()
	mockSync := mock_controller.NewMockSynchronizer(ctrl)
	h := &ApiHttpHandler{Synchronizer: mockSync}
	app.Get("/site/:tenantId", h.GetSite)
	app.Post("/tenant/register", h.RegisterTenant)
	app.Get("/realms", h.GetRealms)
	return app, mockSync, h
}

func TestGetSite(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		mockSetup      func(m *mock_controller.MockSynchronizer)
		expectedStatus int
	}{
		{
			name: "missing tenant id",
			url:  "",
			mockSetup: func(m *mock_controller.MockSynchronizer) {
				// no call expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing URL",
			url:            "",
			mockSetup:      func(m *mock_controller.MockSynchronizer) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "site found",
			url:  "https://ok.com",
			mockSetup: func(m *mock_controller.MockSynchronizer) {
				m.EXPECT().
					GetSite(gomock.Any(), "tenant123", "https://ok.com", true, false).
					Return(`{"site":"ok"}`, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "site not found",
			url:  "https://none.com",
			mockSetup: func(m *mock_controller.MockSynchronizer) {
				m.EXPECT().
					GetSite(gomock.Any(), "tenant123", "https://none.com", true, false).
					Return("", nil)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "synchronizer error",
			url:  "https://fail.com",
			mockSetup: func(m *mock_controller.MockSynchronizer) {
				m.EXPECT().
					GetSite(gomock.Any(), "tenant123", "https://fail.com", true, false).
					Return("", errors.New("sync failed"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			app, mockSync, _ := setupTestHandlers(ctrl)
			tt.mockSetup(mockSync)

			urlPath := "/site/tenant123"
			req := httptest.NewRequest(http.MethodGet, urlPath, nil)
			if tt.url != "" {
				req.Header.Set(URL, tt.url)
			}

			resp, _ := app.Test(req)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestRegisterTenant(t *testing.T) {
	tests := []struct {
		name           string
		body           any
		mockSetup      func(m *mock_controller.MockSynchronizer)
		expectedStatus int
	}{
		{
			name: "invalid JSON body",
			body: "not-json",
			mockSetup: func(m *mock_controller.MockSynchronizer) {
				// no calls expected
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "synchronizer error",
			body: tm.Tenant{ExternalId: "123", TenantName: "TestTenant"},
			mockSetup: func(m *mock_controller.MockSynchronizer) {
				m.EXPECT().
					RegisterTenant(gomock.Any(), tm.Tenant{ExternalId: "123", TenantName: "TestTenant"}).
					Return(errors.New("db failed"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "successful registration",
			body: tm.Tenant{ExternalId: "abc", TenantName: "TenantOK"},
			mockSetup: func(m *mock_controller.MockSynchronizer) {
				m.EXPECT().
					RegisterTenant(gomock.Any(), tm.Tenant{ExternalId: "abc", TenantName: "TenantOK"}).
					Return(nil)
			},
			expectedStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			app, mockSync, _ := setupTestHandlers(ctrl)
			tt.mockSetup(mockSync)

			var req *http.Request
			switch body := tt.body.(type) {
			case string:
				req = httptest.NewRequest(http.MethodPost, "/tenant/register", bytes.NewBufferString(body))
			default:
				data, _ := json.Marshal(body)
				req = httptest.NewRequest(http.MethodPost, "/tenant/register", bytes.NewBuffer(data))
			}
			req.Header.Set("Content-Type", "application/json")

			resp, _ := app.Test(req)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

type PgClientMock struct {
	sqlDb *sql.DB
}

func newPgClientMock(sqlDb *sql.DB) *PgClientMock {
	return &PgClientMock{sqlDb: sqlDb}
}

func (p PgClientMock) GetSqlDb(_ context.Context) (*sql.DB, error) {
	return p.sqlDb, nil
}

func (p PgClientMock) GetBunDb(ctx context.Context) (*bun.DB, error) {
	sqlDb, err := p.GetSqlDb(ctx)
	if err != nil {
		return nil, err
	}
	dbBun := bun.NewDB(sqlDb, pgdialect.New())
	return dbBun, err
}

func TestGetRealmsApi(t *testing.T) {
	serviceloader.Register(0, &security.DummyToken{})
	serviceloader.Register(0, utils.NewResourceGroupAnnotationsMapper("qubership.cloud"))
	idpFacade := idp.NewFacade("test-namespace", idp.DummyRetryableClient{}, logging.GetLogger("idp-facade"))
	app := fiber.New()
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	pgClient := newPgClientMock(db)
	routerDao := pg.NewRouteManager(pgClient)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPmClient := mock_synchronizer.NewMockpaasMediationClient(ctrl)
	mockPmClient.EXPECT().StartSyncingCache(gomock.Any())
	mockPmClient.EXPECT().GetNamespace().Return("test-namespace").AnyTimes()
	mockPmClient.EXPECT().
		GetRoutes(gomock.Any(), gomock.Eq("test-namespace")).
		Return(&[]pmClientDomain.Route{{
			Metadata: pmClientDomain.Metadata{},
			Spec:     pmClientDomain.RouteSpec{},
		}}, nil).
		AnyTimes()
	mockPmClient.EXPECT().
		GetConfigMaps2(gomock.Any(), "test-namespace", gomock.Any()).
		Return(&[]pmClientDomain.Configmap{{
			Metadata: pmClientDomain.ConfigMapMetaData{},
			Data:     pmClientDomain.ConfigMapData{},
		}}, nil).AnyTimes()

	mock.ExpectQuery("SELECT \"init\".\"initialized\" FROM \"inits\" AS \"init\" LIMIT 1").
		WillReturnRows(sqlmock.NewRows([]string{"initialized"}).AddRow(true))

	mockTmClient := mock_synchronizer.NewMocktmClient(ctrl)
	mockTmClient.EXPECT().
		GetActiveTenantsCache(gomock.Any()).
		Return([]tm.Tenant{{
			ObjectId:    "testObjectId",
			ExternalId:  "testExternalId",
			DomainName:  "testDomainName",
			ServiceName: "testServiceName",
			TenantName:  "testTenantName",
			Namespace:   "testNamespace"}},
		).AnyTimes()

	sync := synchronizer.New(routerDao, mockPmClient, mockTmClient, nil, 1000, "testhost", pgClient, idpFacade, false, nil, "testzone", "http")
	h := &ApiHttpHandler{Synchronizer: sync}
	app.Get("/realms", h.GetRealms)

	req := httptest.NewRequest(http.MethodGet, "/realms", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(body), "testExternalId")
}

func TestGet(t *testing.T) {
	serviceloader.Register(0, utils.NewResourceGroupAnnotationsMapper("qubership.cloud"))
	ctrl := gomock.NewController(t)
	app := fiber.New()
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)

	mock.ExpectQuery(`SELECT "init"."initialized" FROM "inits" AS "init" LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"initialized"}).AddRow(true))

	mock.ExpectQuery(`SELECT "tenant_dns"."tenant_id", "tenant_dns"."tenant_admin", "tenant_dns"."sites", "tenant_dns"."active", "tenant_dns"."namespaces", "tenant_dns"."domain_name", "tenant_dns"."service_name", "tenant_dns"."tenant_name", "tenant_dns"."removed" FROM "tenant_dns"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "tenant_admin", "sites", "active", "namespaces",
			"domain_name", "service_name", "tenant_name", "removed",
		}).AddRow("testObjectId", "admin1", "{}", true, "{}", "example.com", "service1", "testTenant", false))

	idpFacade := idp.NewFacade("test-namespace", idp.DummyRetryableClient{}, logging.GetLogger("idp-facade"))
	pgClient := newPgClientMock(db)
	routerDao := pg.NewRouteManager(pgClient)

	mockPmClient := mock_synchronizer.NewMockpaasMediationClient(ctrl)
	mockPmClient.EXPECT().GetNamespace().Return("test-namespace").AnyTimes()
	mockPmClient.EXPECT().
		GetRoutesByFilter(gomock.Any(), "test-namespace", gomock.Any()).
		Return(&[]pmClientDomain.Route{{}}, nil).AnyTimes()
	mockPmClient.EXPECT().StartSyncingCache(gomock.Any())
	mockPmClient.EXPECT().
		GetRoutes(gomock.Any(), gomock.Eq("test-namespace")).
		Return(&[]pmClientDomain.Route{{
			Metadata: pmClientDomain.Metadata{},
			Spec:     pmClientDomain.RouteSpec{},
		}}, nil).
		AnyTimes()
	mockPmClient.EXPECT().
		GetConfigMaps2(gomock.Any(), "test-namespace", gomock.Any()).
		Return(&[]pmClientDomain.Configmap{{
			Metadata: pmClientDomain.ConfigMapMetaData{},
			Data:     pmClientDomain.ConfigMapData{},
		}}, nil).AnyTimes()
	mockPmClient.EXPECT().
		GetRoutesForNamespaces2(gomock.Any(), []string{"test-namespace"}, gomock.Any()).
		Return(&[]pmClientDomain.Route{{}}, nil).AnyTimes()

	mockTmClient := mock_synchronizer.NewMocktmClient(ctrl)
	mockTmClient.EXPECT().
		GetTenantByExternalId(gomock.Any(), "testExternalId").
		Return(&tm.Tenant{
			ObjectId:   "testObjectId",
			ExternalId: "testExternalId",
			TenantName: "testTenant",
		}, nil).AnyTimes()
	mockTmClient.EXPECT().
		GetTenantByObjectId(gomock.Any(), gomock.Any()).
		Return(&tm.Tenant{
			ObjectId:   "testObjectId",
			ExternalId: "testExternalId",
			TenantName: "testTenant",
		}, nil).AnyTimes()
	mockTmClient.EXPECT().
		GetActiveTenantsCache(gomock.Any()).
		Return([]tm.Tenant{{
			ObjectId:    "testObjectId",
			ExternalId:  "testExternalId",
			DomainName:  "testDomainName",
			ServiceName: "testServiceName",
			TenantName:  "testTenantName",
			Namespace:   "testNamespace"}},
		).AnyTimes()
	mockTmClient.EXPECT().
		SubscribeToAll(gomock.Any()).AnyTimes()

	sync := synchronizer.New(routerDao, mockPmClient, mockTmClient, nil, 1000, "testhost", pgClient, idpFacade, false, nil, "testzone", "http")
	h := &ApiHttpHandler{Synchronizer: sync}
	app.Get("routes/:tenantId", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/routes/testTenant", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(body), "{\"tenantId\":\"testObjectId\",\"tenantAdmin\":\"admin1\",\"sites\":{},\"active\":true,\"namespaces\":[],\"domainName\":\"example.com\",\"serviceName\":\"service1\",\"tenantName\":\"testTenant\"}")
}
