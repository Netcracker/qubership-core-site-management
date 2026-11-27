package synchronizer

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/dao/pg"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/domain"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/domain/validator"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/messaging"
	pmClient "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient"
	mdomain "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/tm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

const (
	watchNamespace   = "core-ci-satellite-1"
	watchTenantId    = "tenant-from-watch"
	baselineHost     = "public-gateway." + baselineDomain
	satelliteOwnHost = "app.satellite.example"
	satelliteService = "satellite-app"
)

type watchFixture struct {
	dao              *pg.RouteManagerDao
	sync             *Synchronizer
	deletedRoutes    []string
	createdHosts     []string
	serviceListCalls *atomic.Int32
}

// newWatchFixture builds a synchronizer on a real database and a paas-mediation client that records its requests.
func newWatchFixture(t *testing.T, ctx context.Context, satellite bool) *watchFixture {
	dao := pg.NewRouteManager(newMigratedPgClient(t, ctx))
	fixture := &watchFixture{dao: dao, serviceListCalls: &atomic.Int32{}}
	gateway, err := url.Parse("http://internal-gateway:8080")
	require.NoError(t, err)
	routesUrl := "http://internal-gateway:8080/api/v2/paas-mediation/namespaces/" + watchNamespace + "/routes"
	paasClient := pmClient.NewTestPaasMediationClientWithCaches(gateway, watchNamespace, &pmClient.TestHTTPExecutor{
		StatusCode:     fasthttp.StatusOK,
		StatusByMethod: map[string]int{fasthttp.MethodPost + ":" + routesUrl: fasthttp.StatusCreated},
		OnRequest: func(method, requestUrl string, body []byte) {
			switch method {
			case fasthttp.MethodDelete:
				fixture.deletedRoutes = append(fixture.deletedRoutes, requestUrl)
			case fasthttp.MethodPost:
				var route mdomain.Route
				require.NoError(t, json.Unmarshal(body, &route))
				fixture.createdHosts = append(fixture.createdHosts, route.Spec.Host)
			}
		},
	})
	fixture.sync = &Synchronizer{
		dao:                    dao,
		pmClient:               paasClient,
		mailSender:             &messaging.MailSender{},
		schemeValidator:        validator.NewSchemeValidator(),
		compositeSatelliteMode: satellite,
		servicesGetter: func(_ context.Context, _ string, _ func(*mdomain.Service) bool) (*[]mdomain.Service, error) {
			fixture.serviceListCalls.Add(1)
			return &[]mdomain.Service{{Metadata: mdomain.Metadata{Name: satelliteService, Namespace: watchNamespace}}}, nil
		},
	}
	return fixture
}

func activeTenantEvent() tm.Tenant {
	return tm.Tenant{
		ObjectId:   watchTenantId,
		ExternalId: "external-id",
		TenantName: "autotest-1",
		DomainName: baselineDomain,
		Status:     domain.Active,
	}
}

func storedTenantWithBaselineAndOwnHost() domain.TenantDns {
	return domain.TenantDns{
		TenantId:   watchTenantId,
		TenantName: "autotest-1",
		DomainName: baselineDomain,
		Active:     true,
		Sites: domain.Sites{"default": domain.Services{
			"public-gateway": domain.AddressList{domain.Address("https://" + baselineHost + "/")},
			satelliteService: domain.AddressList{satelliteOwnHost},
		}},
	}
}

// A satellite that stores an active tenant from the watch drops the hosts of the baseline domain, keeps its own
// routes, and keeps recording the domain it was sent.
func TestUpsertTenantFromTM_Satellite_DropsBaselineHostsAndKeepsOwnRoutes(t *testing.T) {
	ctx := context.Background()
	f := newWatchFixture(t, ctx, true)
	require.NoError(t, f.dao.Upsert(ctx, storedTenantWithBaselineAndOwnHost()))

	require.NoError(t, f.sync.upsertTenantFromTM(ctx, activeTenantEvent()))

	stored, err := f.dao.FindByTenantId(ctx, watchTenantId)
	require.NoError(t, err)
	assert.True(t, stored.Active)
	assert.Equal(t, baselineDomain, stored.DomainName)
	assert.Equal(t, domain.Sites{"default": domain.Services{
		satelliteService: domain.AddressList{satelliteOwnHost},
	}}, stored.Sites)
}

// A tenant that is new to a satellite gets no default routes from the baseline domain.
func TestUpsertTenantFromTM_Satellite_NewActiveTenantGetsNoGeneratedRoutes(t *testing.T) {
	ctx := context.Background()
	f := newWatchFixture(t, ctx, true)

	require.NoError(t, f.sync.upsertTenantFromTM(ctx, activeTenantEvent()))

	stored, err := f.dao.FindByTenantId(ctx, watchTenantId)
	require.NoError(t, err)
	assert.True(t, stored.Active)
	assert.Empty(t, stored.Sites)
	assert.Zero(t, f.serviceListCalls.Load(), "the satellite must not list services to generate tenant hosts")
}

// A route that an earlier version created on the satellite for the baseline host is deleted by the next sync pass.
// The route of the satellite's own host is kept, and the pass creates no route for the baseline host.
func TestUpsertTenantFromTM_Satellite_NextSyncDeletesTheRouteOfTheBaselineHost(t *testing.T) {
	ctx := context.Background()
	f := newWatchFixture(t, ctx, true)
	require.NoError(t, f.dao.Upsert(ctx, storedTenantWithBaselineAndOwnHost()))
	staleRoute := mdomain.Route{
		Metadata: mdomain.Metadata{
			Kind:        mdomain.RouteKindHTTP,
			Name:        "public-gateway-service-" + watchTenantId,
			Namespace:   watchNamespace,
			Annotations: map[string]string{"netcracker.cloud/tenant.service.tenant.id": watchTenantId},
		},
		Spec: mdomain.RouteSpec{Host: baselineHost, Service: mdomain.Target{Name: "public-gateway-service"}},
	}
	pmClient.RoutesMap(f.sync.pmClient, watchNamespace)[staleRoute.CacheKey()] = staleRoute
	require.NoError(t, f.sync.upsertTenantFromTM(ctx, activeTenantEvent()))

	require.NoError(t, f.sync.processSynchronization(ctx))

	require.Len(t, f.deletedRoutes, 1, "deleted routes")
	assert.True(t, strings.Contains(f.deletedRoutes[0], staleRoute.Metadata.Name), "deleted route: %s", f.deletedRoutes[0])
	assert.Equal(t, []string{satelliteOwnHost}, f.createdHosts, "hosts of the routes the pass created")
}

// The baseline is not changed by the guard: it keeps the sites it has stored for an active tenant.
func TestUpsertTenantFromTM_Baseline_KeepsStoredSites(t *testing.T) {
	ctx := context.Background()
	f := newWatchFixture(t, ctx, false)
	stored := storedTenantWithBaselineAndOwnHost()
	require.NoError(t, f.dao.Upsert(ctx, stored))

	require.NoError(t, f.sync.upsertTenantFromTM(ctx, activeTenantEvent()))

	after, err := f.dao.FindByTenantId(ctx, watchTenantId)
	require.NoError(t, err)
	assert.Equal(t, stored.Sites, after.Sites)
}
