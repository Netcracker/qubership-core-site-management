package synchronizer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/dao/pg"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/domain"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/messaging"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/migration"
	pmClient "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient"
	mdomain "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/testharness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/valyala/fasthttp"
)

// A tenant that is restored while the sync pass waits on paas-mediation to delete
// routes keeps its database row, and the same pass creates its route. The pass used
// to delete the row by the snapshot it read at the start, so no later pass created
// the route, and TenantSchemeIT.testActivateTenant timed out waiting for the HTTPRoute.
func TestProcessSynchronization_TenantRestoredDuringRouteDeletion_KeepsRowAndGetsRoute(t *testing.T) {
	ctx := context.Background()
	const (
		namespace = "test-namespace"
		tenantId  = "restored-tenant"
		service   = "frontend"
		host      = "restored.example.com"
		routesUrl = "http://internal-gateway:8080/api/v2/paas-mediation/namespaces/test-namespace/routes"
	)
	dao := pg.NewRouteManager(newMigratedPgClient(t, ctx))
	require.NoError(t, dao.SaveTenant(ctx, &domain.TenantDns{TenantId: tenantId, Active: true, Removed: true}))

	var createdHosts []string
	gateway, err := url.Parse("http://internal-gateway:8080")
	require.NoError(t, err)
	paasClient := pmClient.NewTestPaasMediationClientWithCaches(gateway, namespace, &pmClient.TestHTTPExecutor{
		StatusCode:     fasthttp.StatusOK,
		StatusByMethod: map[string]int{fasthttp.MethodPost + ":" + routesUrl: fasthttp.StatusCreated},
		OnRequest: func(method, url string, body []byte) {
			switch method {
			case fasthttp.MethodDelete:
				restored := domain.TenantDns{
					TenantId: tenantId,
					Active:   true,
					Sites:    domain.Sites{"default": domain.Services{service: domain.AddressList{host}}},
				}
				require.NoError(t, dao.Upsert(ctx, restored))
			case fasthttp.MethodPost:
				var route mdomain.Route
				require.NoError(t, json.Unmarshal(body, &route))
				createdHosts = append(createdHosts, route.Spec.Host)
			}
		},
	})
	staleRoute := mdomain.Route{
		Metadata: mdomain.Metadata{
			Kind:        mdomain.RouteKindHTTP,
			Name:        "frontend-restored-tenant",
			Namespace:   namespace,
			Annotations: map[string]string{"netcracker.cloud/tenant.service.tenant.id": tenantId},
		},
		Spec: mdomain.RouteSpec{Host: host, Service: mdomain.Target{Name: service}},
	}
	pmClient.RoutesMap(paasClient, namespace)[staleRoute.CacheKey()] = staleRoute
	sync := Synchronizer{
		dao:        dao,
		pmClient:   paasClient,
		mailSender: &messaging.MailSender{},
		servicesGetter: func(_ context.Context, _ string, _ func(*mdomain.Service) bool) (*[]mdomain.Service, error) {
			return &[]mdomain.Service{{Metadata: mdomain.Metadata{Name: service, Namespace: namespace}}}, nil
		},
	}

	require.NoError(t, sync.processSynchronization(ctx))

	tenant, err := dao.FindByTenantId(ctx, tenantId)
	if assert.NoError(t, err, "FindByTenantId(%q) after the pass", tenantId) {
		assert.False(t, tenant.Removed, "removed flag of the restored tenant")
	}
	assert.Equal(t, []string{host}, createdHosts, "hosts of the routes the pass created")
}

func newMigratedPgClient(t *testing.T, ctx context.Context) *bunPgClient {
	container := testharness.PreparePostgres(t, ctx)
	require.NotNil(t, container)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err)
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable&connect_timeout=5",
		testharness.PostgresUsername, testharness.PostgresPassword, endpoint, testharness.PostgresDatabase)
	sqlDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return sqlDB.PingContext(ctx) == nil }, time.Minute, time.Second,
		"Postgres at %s did not answer a ping", endpoint)
	db := bun.NewDB(sqlDB, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, migration.RunMigration(db, ctx))
	return &bunPgClient{db: db}
}

type bunPgClient struct {
	db *bun.DB
}

func (c *bunPgClient) GetSqlDb(context.Context) (*sql.DB, error) {
	return c.db.DB, nil
}

func (c *bunPgClient) GetBunDb(context.Context) (*bun.DB, error) {
	return c.db, nil
}
