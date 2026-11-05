package synchronizer

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/domain"
	pmClient "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient"
	mdomain "github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestDeleteRemovedTenants_DeletesOnlyRemoved(t *testing.T) {
	var deleted []string
	tenants := []domain.TenantDns{
		{TenantId: "keep", Removed: false},
		{TenantId: "gone", Removed: true},
	}

	deleteRemovedTenants(context.Background(), tenants, func(_ context.Context, tenantId string) error {
		deleted = append(deleted, tenantId)
		return nil
	})

	assert.Equal(t, []string{"gone"}, deleted)
}

func TestDeleteRemovedTenants_LogsErrorAndContinues(t *testing.T) {
	var deleted []string
	tenants := []domain.TenantDns{
		{TenantId: "first", Removed: true},
		{TenantId: "second", Removed: true},
	}

	deleteRemovedTenants(context.Background(), tenants, func(_ context.Context, tenantId string) error {
		deleted = append(deleted, tenantId)
		if tenantId == "first" {
			return errors.New("delete failed")
		}
		return nil
	})

	assert.Equal(t, []string{"first", "second"}, deleted)
}

func TestDeleteVirtualService_PassesRouteKindOnDelete(t *testing.T) {
	namespace := "test-namespace"
	serviceName := "virtual-service"
	virtualService := mdomain.Service{
		Metadata: mdomain.Metadata{
			Name:      serviceName,
			Namespace: namespace,
			Annotations: map[string]string{
				"netcracker.cloud/tenant.service.type": "virtual",
			},
		},
	}
	httpRoute := mdomain.Route{
		Metadata: mdomain.Metadata{
			Kind:      mdomain.RouteKindHTTP,
			Name:      "cloud-administrator",
			Namespace: namespace,
		},
		Spec: mdomain.RouteSpec{
			Service: mdomain.Target{Name: serviceName},
		},
	}
	grpcRoute := mdomain.Route{
		Metadata: mdomain.Metadata{
			Kind:      mdomain.RouteKindGRPC,
			Name:      "cloud-administrator",
			Namespace: namespace,
		},
		Spec: mdomain.RouteSpec{
			Service: mdomain.Target{Name: "other-service"},
		},
	}

	gateway, err := url.Parse("http://internal-gateway:8080")
	require.NoError(t, err)
	paasClient := pmClient.NewTestPaasMediationClientWithCaches(gateway, namespace, &pmClient.TestHTTPExecutor{
		StatusCode: fasthttp.StatusOK,
		StatusByMethod: map[string]int{
			fasthttp.MethodDelete + ":http://internal-gateway:8080/api/v2/paas-mediation/namespaces/test-namespace/routes/cloud-administrator": fasthttp.StatusInternalServerError,
		},
	})
	pmClient.ServicesMap(paasClient, namespace)[serviceName] = virtualService
	routesMap := pmClient.RoutesMap(paasClient, namespace)
	routesMap[httpRoute.CacheKey()] = httpRoute
	routesMap[grpcRoute.CacheKey()] = grpcRoute

	sync := Synchronizer{pmClient: paasClient}
	err = sync.DeleteVirtualService(context.Background(), serviceName)

	assert.Error(t, err)
	assert.Len(t, routesMap, 2)
	_, httpStillPresent := routesMap[httpRoute.CacheKey()]
	_, grpcStillPresent := routesMap[grpcRoute.CacheKey()]
	assert.True(t, httpStillPresent)
	assert.True(t, grpcStillPresent)
}

func TestDeleteVirtualService_RemovesRouteFromCacheByKind(t *testing.T) {
	namespace := "test-namespace"
	serviceName := "virtual-service"
	virtualService := mdomain.Service{
		Metadata: mdomain.Metadata{
			Name:      serviceName,
			Namespace: namespace,
			Annotations: map[string]string{
				"netcracker.cloud/tenant.service.type": "virtual",
			},
		},
	}
	httpRoute := mdomain.Route{
		Metadata: mdomain.Metadata{
			Kind:      mdomain.RouteKindHTTP,
			Name:      "cloud-administrator",
			Namespace: namespace,
		},
		Spec: mdomain.RouteSpec{
			Service: mdomain.Target{Name: serviceName},
		},
	}
	grpcRoute := mdomain.Route{
		Metadata: mdomain.Metadata{
			Kind:      mdomain.RouteKindGRPC,
			Name:      "cloud-administrator",
			Namespace: namespace,
		},
		Spec: mdomain.RouteSpec{
			Service: mdomain.Target{Name: "other-service"},
		},
	}

	gateway, err := url.Parse("http://internal-gateway:8080")
	require.NoError(t, err)
	paasClient := pmClient.NewTestPaasMediationClientWithCaches(gateway, namespace, &pmClient.TestHTTPExecutor{StatusCode: fasthttp.StatusOK})
	pmClient.ServicesMap(paasClient, namespace)[serviceName] = virtualService
	routesMap := pmClient.RoutesMap(paasClient, namespace)
	routesMap[httpRoute.CacheKey()] = httpRoute
	routesMap[grpcRoute.CacheKey()] = grpcRoute

	sync := Synchronizer{pmClient: paasClient}

	var panicValue any
	func() {
		defer func() {
			panicValue = recover()
		}()
		_ = sync.DeleteVirtualService(context.Background(), serviceName)
	}()

	require.NotNil(t, panicValue, "expected panic from nil dao after route cache update")
	assert.Len(t, routesMap, 1)
	_, httpStillPresent := routesMap[httpRoute.CacheKey()]
	_, grpcStillPresent := routesMap[grpcRoute.CacheKey()]
	assert.False(t, httpStillPresent)
	assert.True(t, grpcStillPresent)
}

func TestRefreshSettings_ReturnsFreshTenants(t *testing.T) {
	snapshot := &[]domain.TenantDns{{TenantId: "t", Removed: true}}
	fresh := &[]domain.TenantDns{{TenantId: "t", Removed: false}}

	result := refreshSettings(context.Background(), func(context.Context) (*[]domain.TenantDns, error) {
		return fresh, nil
	}, snapshot)

	assert.Same(t, fresh, result)
}

func TestRefreshSettings_KeepsSnapshotOnError(t *testing.T) {
	snapshot := &[]domain.TenantDns{{TenantId: "t", Removed: true}}

	result := refreshSettings(context.Background(), func(context.Context) (*[]domain.TenantDns, error) {
		return nil, errors.New("db down")
	}, snapshot)

	assert.Same(t, snapshot, result)
}
