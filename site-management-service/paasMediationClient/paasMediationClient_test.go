package paasMediationClient

import (
	"context"
	"encoding/json"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
	. "github.com/smarty/assertions"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type (
	fakeHttpExecutor struct {
		response                          *fasthttp.Response
		isCreateSecureRequestMethodCalled bool
		requestUrl                        string
	}
)

func (fakeHttpExecutor *fakeHttpExecutor) doRequest(_ context.Context, url string, _ string, _ []byte) (*fasthttp.Response, error) {
	fakeHttpExecutor.isCreateSecureRequestMethodCalled = true
	fakeHttpExecutor.requestUrl = url
	return fakeHttpExecutor.response, nil
}

func newFakeHttpExecutor(responseBody interface{}, httpCode int) *fakeHttpExecutor {
	httpExecutor := fakeHttpExecutor{}
	response := fasthttp.AcquireResponse()
	responseByte, _ := json.Marshal(responseBody)
	response.SetBody(responseByte)
	response.SetStatusCode(httpCode)
	httpExecutor.response = response
	httpExecutor.isCreateSecureRequestMethodCalled = false
	return &httpExecutor
}

func TestUpdateRoutesCacheByCreateEvent(t *testing.T) {
	compositeCacheTest := &CompositeCache{
		routesCache: &RoutesCache{
			mutex: &sync.RWMutex{},
			routes: map[string]*map[string]domain.Route{
				"test-namespace": {},
			},
		}}
	routeUpdateEvent := RouteUpdate{
		Type:        updateTypeAdded,
		RouteObject: domain.Route{Metadata: domain.Metadata{Namespace: "test-namespace"}},
	}
	compositeCacheTest.updateRoutesCache(context.Background(), &routeUpdateEvent)
	assertResult(So(compositeCacheTest.routesCache.routes, ShouldHaveLength, 1))
}

func TestUpdateRoutesCacheByDeleteEvent(t *testing.T) {
	compositeCacheTest := &CompositeCache{
		routesCache: &RoutesCache{
			mutex: &sync.RWMutex{},
			routes: map[string]*map[string]domain.Route{
				"test-namespace": {
					"route-one": {},
					"route-two": {},
				},
			},
		}}
	routeEvent := RouteUpdate{
		Type:        updateTypeDeleted,
		RouteObject: domain.Route{Metadata: domain.Metadata{Namespace: "test-namespace", Name: "route-one"}},
	}
	compositeCacheTest.updateRoutesCache(context.Background(), &routeEvent)
	assertResult(So(compositeCacheTest.routesCache.routes, ShouldHaveLength, 1))
}

func TestUpdateRoutesCacheByInitEvent(t *testing.T) {

	oneRoute := domain.Route{Metadata: domain.Metadata{Name: "route-one"}}
	twoRoute := domain.Route{Metadata: domain.Metadata{Name: "route-two"}}
	routes := []domain.Route{
		oneRoute,
		twoRoute,
	}

	fakeExec := newFakeHttpExecutor(&routes, 200)

	paasClient := &PaasMediationClient{httpExecutor: fakeExec}
	compositeCacheTest := &CompositeCache{
		routesCache: &RoutesCache{
			mutex: &sync.RWMutex{},
			routes: map[string]*map[string]domain.Route{
				"test-namespace": {
					"route-one":   {},
					"route-two":   {},
					"route-three": {},
				},
			},
			initCache: paasClient.initRoutesMapInCache,
		}}
	paasClient.cache = compositeCacheTest
	routeEvent := RouteUpdate{
		Type:        updateTypeInit,
		RouteObject: domain.Route{Metadata: domain.Metadata{Namespace: "test-namespace"}},
	}
	paasClient.cache.updateRoutesCache(context.Background(), &routeEvent)

	routesRes := *compositeCacheTest.routesCache.routes["test-namespace"]
	assertResult(So(fakeExec.isCreateSecureRequestMethodCalled, ShouldBeTrue))
	assertResult(So(routesRes, ShouldHaveLength, 2))
	for _, route := range []domain.Route{oneRoute, twoRoute} {
		_, ok := routesRes[route.Metadata.Name]
		assert.True(t, ok)
	}
}

func TestUpdateRoutesCacheByInitEventNewNamespace(t *testing.T) {

	oneRoute := domain.Route{Metadata: domain.Metadata{Name: "route-three"}}
	twoRoute := domain.Route{Metadata: domain.Metadata{Name: "route-four"}}
	routes := []domain.Route{
		oneRoute,
		twoRoute,
	}

	fakeExec := newFakeHttpExecutor(&routes, 200)

	paasClient := &PaasMediationClient{httpExecutor: fakeExec}
	compositeCacheTest := &CompositeCache{
		routesCache: &RoutesCache{
			mutex: &sync.RWMutex{},
			routes: map[string]*map[string]domain.Route{
				"test-namespace": {
					"route-one":   {},
					"route-two":   {},
					"route-three": {},
				},
			},
			initCache: paasClient.initRoutesMapInCache,
		}}
	paasClient.cache = compositeCacheTest
	routeEvent := RouteUpdate{
		Type:        updateTypeInit,
		RouteObject: domain.Route{Metadata: domain.Metadata{Namespace: "new-test-namespace"}},
	}
	paasClient.cache.updateRoutesCache(context.Background(), &routeEvent)

	routesRes := *compositeCacheTest.routesCache.routes["new-test-namespace"]
	assertResult(So(fakeExec.isCreateSecureRequestMethodCalled, ShouldBeTrue))
	assertResult(So(routesRes, ShouldHaveLength, 2))
	for _, route := range []domain.Route{oneRoute, twoRoute} {
		_, ok := routesRes[route.Metadata.Name]
		assert.True(t, ok)
	}
}

func TestCreateRoute(t *testing.T) {
	route := domain.Route{Metadata: domain.Metadata{Name: "route-one", Namespace: "test-namespace"}}
	httpExecutor := newFakeHttpExecutor(&route, 201)
	internalGateway, e := url.Parse("http://internal-gateway:8080")
	if e != nil {
		panic(e)
	}
	paasClient := createPaasClientWithRouteCache(httpExecutor, internalGateway)
	err := paasClient.CreateRoute(context.Background(), &route, "test-namespace")
	if err != nil {
		panic(err)
	}
	assertResult(So(httpExecutor.isCreateSecureRequestMethodCalled, ShouldBeTrue))
	assertResult(So(httpExecutor.requestUrl, ShouldEqual, "http://internal-gateway:8080/api/v2/paas-mediation/namespaces/test-namespace/routes"))
	assert.Equal(t, 1, len(*paasClient.cache.routesCache.routes["test-namespace"]))
}

func TestDeleteRoute(t *testing.T) {
	routeName := "route-one"
	httpExecutor := newFakeHttpExecutor(nil, 200)
	internalGateway, e := url.Parse("http://internal-gateway:8080")
	if e != nil {
		panic(e)
	}
	paasClient := createPaasClientWithRouteCache(httpExecutor, internalGateway)
	err := paasClient.DeleteRoute(context.Background(), "test-namespace", routeName)
	if err != nil {
		panic(err)
	}
	assertResult(So(httpExecutor.isCreateSecureRequestMethodCalled, ShouldBeTrue))
	assertResult(So(httpExecutor.requestUrl, ShouldEqual, "http://internal-gateway:8080/api/v2/paas-mediation/namespaces/test-namespace/routes/"+routeName))
	assert.Equal(t, 0, len(*paasClient.cache.routesCache.routes["test-namespace"]))
}

func createPaasClientWithRouteCache(httpExecutor *fakeHttpExecutor, gateway *url.URL) *PaasMediationClient {
	paasClient := &PaasMediationClient{httpExecutor: httpExecutor, InternalGatewayAddress: gateway}
	initialRoutes := make(map[string]*map[string]domain.Route)
	initialNamespace := make(map[string]domain.Route)
	initialRoutes["test-namespace"] = &initialNamespace
	paasClient.cache = &CompositeCache{
		routesCache: &RoutesCache{
			mutex:  &sync.RWMutex{},
			routes: initialRoutes,
		},
	}
	return paasClient
}

func TestBuildURL(t *testing.T) {
	namespace := "test-namespace"
	resource := "routes"
	resourceName := "route-one"
	internalGateway, e := url.Parse("http://internal-gateway:8080")
	if e != nil {
		panic(e)
	}
	paasClient := &PaasMediationClient{InternalGatewayAddress: internalGateway}
	_, err := paasClient.buildUrl(context.Background(), namespace, "", "")
	assertResult(So(err, ShouldNotBeNil))

	requestedUrl, err := paasClient.buildUrl(context.Background(), namespace, resource, "")
	assertResult(So(err, ShouldBeNil))
	assertResult(So(requestedUrl, ShouldEqual, "http://internal-gateway:8080/api/v2/paas-mediation/namespaces/test-namespace/routes"))

	requestedUrl, err = paasClient.buildUrl(context.Background(), namespace, resource, resourceName)
	assertResult(So(err, ShouldBeNil))
	assertResult(So(requestedUrl, ShouldEqual, "http://internal-gateway:8080/api/v2/paas-mediation/namespaces/test-namespace/routes/route-one"))

}

func TestSyncingCache(t *testing.T) {
	paasClient := &PaasMediationClient{}

	initialRoutes := make(map[string]*map[string]domain.Route)
	initialRoutesNamespace := make(map[string]domain.Route)
	initialRoutes["test-namespace"] = &initialRoutesNamespace
	routesChannel := make(chan []byte, 50)

	initialServices := make(map[string]*map[string]domain.Service)
	initialServicesNamespace := make(map[string]domain.Service)
	initialServices["test-namespace"] = &initialServicesNamespace
	servicesChannel := make(chan []byte, 50)

	initialConfigMaps := make(map[string]*map[string]domain.Configmap)
	initialConfigMapsNamespace := make(map[string]domain.Configmap)
	initialConfigMaps["test-namespace"] = &initialConfigMapsNamespace
	configMapsChannel := make(chan []byte, 50)

	paasClient.cache = &CompositeCache{
		routesCache: &RoutesCache{
			mutex:  &sync.RWMutex{},
			routes: initialRoutes,
			bus:    routesChannel,
		},
		servicesCache: &ServicesCache{
			mutex:    &sync.RWMutex{},
			services: initialServices,
			bus:      servicesChannel,
		},
		configMapsCache: &ConfigMapsCache{
			mutex:      &sync.RWMutex{},
			configMaps: initialConfigMaps,
			bus:        configMapsChannel,
		},
	}

	paasClient.StartSyncingCache(context.Background())
	routesChannel <- []byte("{\"type\":\"ADDED\",\"object\":{\"metadata\":{\"kind\":\"Route\",\"name\":\"domain-resolver-frontend\",\"namespace\":\"test-namespace\",\"annotations\":{\"kubectl.kubernetes.io/last-applied-configuration\":\"{\\\"apiVersion\\\":\\\"extensions/v1beta1\\\",\\\"kind\\\":\\\"Ingress\\\",\\\"metadata\\\":{\\\"annotations\\\":{\\\"qubership.cloud/tenant.service.show.description\\\":\\\"domain-resolver-frontend\\\",\\\"qubership.cloud/tenant.service.show.name\\\":\\\"Domain resolver frontend\\\",\\\"qubership.cloud/tenant.service.tenant.id\\\":\\\"GENERAL\\\"},\\\"name\\\":\\\"domain-resolver-frontend\\\",\\\"namespace\\\":\\\"test-namespace\\\"},\\\"spec\\\":{\\\"rules\\\":[{\\\"host\\\":\\\"domain-resolver-frontend-test-namespace.cloud.qubership.org\\\",\\\"http\\\":{\\\"paths\\\":[{\\\"backend\\\":{\\\"serviceName\\\":\\\"domain-resolver-frontend\\\",\\\"servicePort\\\":\\\"web\\\"},\\\"path\\\":\\\"/\\\"}]}}]}}\\n\",\"qubership.cloud/tenant.service.show.description\":\"domain-resolver-frontend\",\"qubership.cloud/tenant.service.show.name\":\"Domain resolver frontend\",\"qubership.cloud/tenant.service.tenant.id\":\"GENERAL\"}},\"spec\":{\"host\":\"domain-resolver-frontend-test-namespace.cloud.qubership.org\",\"path\":\"/\",\"to\":{\"name\":\"domain-resolver-frontend\"},\"port\":{\"targetPort\":0}}}}")
	servicesChannel <- []byte("{\"type\":\"ADDED\",\"object\":{\"metadata\":{\"kind\":\"Service\",\"name\":\"public-gateway-service\",\"namespace\":\"test-namespace\",\"annotations\":{\"kubectl.kubernetes.io/last-applied-configuration\":\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Service\\\",\\\"metadata\\\":{\\\"annotations\\\":{\\\"qubership.cloud/start.stage\\\":\\\"1\\\",\\\"qubership.cloud/tenant.service.alias.prefix\\\":\\\"public-gateway\\\",\\\"qubership.cloud/tenant.service.show.description\\\":\\\"Api Gateway to access public API\\\",\\\"qubership.cloud/tenant.service.show.name\\\":\\\"Public Gateway\\\"},\\\"name\\\":\\\"public-gateway-service\\\",\\\"namespace\\\":\\\"test-namespace\\\"},\\\"spec\\\":{\\\"ports\\\":[{\\\"name\\\":\\\"web\\\",\\\"port\\\":8080,\\\"targetPort\\\":8080}],\\\"selector\\\":{\\\"name\\\":\\\"public-frontend-gateway\\\"}}}\\n\",\"qubership.cloud/start.stage\":\"1\",\"qubership.cloud/tenant.service.alias.prefix\":\"public-gateway\",\"qubership.cloud/tenant.service.show.description\":\"Api Gateway to access public API\",\"qubership.cloud/tenant.service.show.name\":\"Public Gateway\"}},\"spec\":{\"ports\":[{\"name\":\"web\",\"protocol\":\"TCP\",\"port\":8080,\"targetPort\":8080}],\"selector\":{\"name\":\"public-frontend-gateway\"},\"clusterIP\":\"172.31.107.233\",\"type\":\"ClusterIP\"}}}")
	configMapsChannel <- []byte("{\"type\":\"ADDED\",\"object\":{\"metadata\":{\"kind\":\"ConfigMap\",\"name\":\"tenant-manager-configs\",\"namespace\":\"test-namespace\",\"annotations\":{\"kubectl.kubernetes.io/last-applied-configuration\":\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"data\\\":{\\\"common-external-routes.json\\\":\\\"[\\\"localhost:4200\\\"]},\\\"kind\\\":\\\"ConfigMap\\\",\\\"metadata\\\":{\\\"annotations\\\":{},\\\"name\\\":\\\"tenant-manager-configs\\\",\\\"namespace\\\":\\\"test-namespace\\\"}}\\n\"}},\"data\":{\"common-external-routes.json\":\"[\\\"localhost:4200\\\"]\"}}}")
	configMapsChannel <- []byte("{\"type\":\"ADDED\",\"object\":{\"metadata\":{\"kind\":\"ConfigMap\",\"name\":\"baseline-version\",\"namespace\":\"test-namespace\",\"annotations\":{\"kubectl.kubernetes.io/last-applied-configuration\":\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"data\\\":{\\\"common-external-routes.json\\\":\\\"[\\\"localhost:4200\\\"]},\\\"kind\\\":\\\"ConfigMap\\\",\\\"metadata\\\":{\\\"annotations\\\":{},\\\"name\\\":\\\"baseline-version\\\",\\\"namespace\\\":\\\"test-namespace\\\"}}\\n\"}},\"data\":{\"common-external-routes.json\":\"[\\\"localhost:4200\\\"]\"}}}")
	configMapsChannel <- []byte("{\"type\":\"ADDED\",\"object\":{\"metadata\":{\"kind\":\"ConfigMap\",\"name\":\"junk-config-map\",\"namespace\":\"test-namespace\",\"annotations\":{\"kubectl.kubernetes.io/last-applied-configuration\":\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"data\\\":{\\\"common-external-routes.json\\\":\\\"[\\\"localhost:4200\\\"]},\\\"kind\\\":\\\"ConfigMap\\\",\\\"metadata\\\":{\\\"annotations\\\":{},\\\"name\\\":\\\"junk-config-map\\\",\\\"namespace\\\":\\\"test-namespace\\\"}}\\n\"}},\"data\":{\"common-external-routes.json\":\"[\\\"localhost:4200\\\"]\"}}}")
	time.Sleep(5 * time.Second)

	assert.Equal(t, "domain-resolver-frontend", (*paasClient.cache.routesCache.routes["test-namespace"])["domain-resolver-frontend"].Metadata.Name)
	assert.Equal(t, "public-gateway-service", (*paasClient.cache.servicesCache.services["test-namespace"])["public-gateway-service"].Metadata.Name)
	assert.Equal(t, "tenant-manager-configs", (*paasClient.cache.configMapsCache.configMaps["test-namespace"])["tenant-manager-configs"].Metadata.Name)
	assert.Equal(t, "baseline-version", (*paasClient.cache.configMapsCache.configMaps["test-namespace"])["baseline-version"].Metadata.Name)
	assert.NotContains(t, *paasClient.cache.configMapsCache.configMaps["test-namespace"], "junk-config-map")
}

func TestSyncingCacheWithError(t *testing.T) {
	routesChannel := make(chan []byte, 50)

	routeUpd := CommonUpdateStr{updateCacheWithPanic, RouteUpdate{}, routesChannel, make(chan time.Time)}

	go func() {
		for {
			routesChannel <- []byte("{\"type\":\"ADDED\",\"object\":{\"metadata\":{\"kind\":\"Route\",\"name\":\"domain-resolver-frontend\",\"namespace\":\"test-namespace-no\",\"annotations\":{\"kubectl.kubernetes.io/last-applied-configuration\":\"{\\\"apiVersion\\\":\\\"extensions/v1beta1\\\",\\\"kind\\\":\\\"Ingress\\\",\\\"metadata\\\":{\\\"annotations\\\":{\\\"qubership.cloud/tenant.service.show.description\\\":\\\"domain-resolver-frontend\\\",\\\"qubership.cloud/tenant.service.show.name\\\":\\\"Domain resolver frontend\\\",\\\"qubership.cloud/tenant.service.tenant.id\\\":\\\"GENERAL\\\"},\\\"name\\\":\\\"domain-resolver-frontend\\\",\\\"namespace\\\":\\\"test-namespace\\\"},\\\"spec\\\":{\\\"rules\\\":[{\\\"host\\\":\\\"domain-resolver-frontend-test-namespace.cloud.qubership.org\\\",\\\"http\\\":{\\\"paths\\\":[{\\\"backend\\\":{\\\"serviceName\\\":\\\"domain-resolver-frontend\\\",\\\"servicePort\\\":\\\"web\\\"},\\\"path\\\":\\\"/\\\"}]}}]}}\\n\",\"qubership.cloud/tenant.service.show.description\":\"domain-resolver-frontend\",\"qubership.cloud/tenant.service.show.name\":\"Domain resolver frontend\",\"qubership.cloud/tenant.service.tenant.id\":\"GENERAL\"}},\"spec\":{\"host\":\"domain-resolver-frontend-test-namespace.cloud.qubership.org\",\"path\":\"/\",\"to\":{\"name\":\"domain-resolver-frontend\"},\"port\":{\"targetPort\":0}}}}")
		}
	}()

	assert.Panics(t, func() {
		syncingCacheInternal(context.Background(), routeUpd, time.Second)
	})
}

func TestFilterRequiredConfigMaps(t *testing.T) {
	assert.True(t, FilterRequiredConfigMaps("tenant-manager-configs"))
	assert.True(t, FilterRequiredConfigMaps("baseline-version"))
	assert.False(t, FilterRequiredConfigMaps("junk-config-map"))
}

func strPtr(s string) *string               { return &s }
func portPtr(i int32) *gatewayv1.PortNumber { p := gatewayv1.PortNumber(i); return &p }

func TestConvertHTTPRoutes_BasicAndEdgeCases(t *testing.T) {
	r := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "http-one",
			Namespace:   "ns",
			Annotations: map[string]string{"a": "b"},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{"host.example"},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{Path: &gatewayv1.HTTPPathMatch{Value: strPtr("/foo")}},
					},
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: "svc-a", Port: portPtr(8080)}}},
						{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: "svc-b"}}},
					},
				},
			},
		},
	}

	rNoHost := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "http-two", Namespace: "ns"},
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{{
				Matches:     []gatewayv1.HTTPRouteMatch{{}},
				BackendRefs: []gatewayv1.HTTPBackendRef{{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: "svc-c", Port: portPtr(80)}}}},
			}},
		},
	}

	got := convertHTTPRoutes([]gatewayv1.HTTPRoute{r, rNoHost})

	assert.Equal(t, 3, len(got))

	assert.Equal(t, "host.example", got[0].Spec.Host)
	assert.Equal(t, "/foo", got[0].Spec.Path)
	assert.Equal(t, "svc-a", got[0].Spec.Service.Name)
	assert.Equal(t, int32(8080), got[0].Spec.Port.TargetPort)
	assert.Equal(t, "http-one", got[0].Metadata.Name)
	assert.Equal(t, "ns", got[0].Metadata.Namespace)
	assert.Equal(t, "b", got[0].Metadata.Annotations["a"])

	assert.Equal(t, "svc-b", got[1].Spec.Service.Name)
	assert.Equal(t, int32(8080), got[1].Spec.Port.TargetPort)

	assert.Equal(t, "", got[2].Spec.Host)
	assert.Equal(t, "svc-c", got[2].Spec.Service.Name)
	assert.Equal(t, int32(80), got[2].Spec.Port.TargetPort)
}

func TestConvertGRPCRoutes_Basic(t *testing.T) {
	gr := gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grpc-one",
			Namespace: "ns",
		},
		Spec: gatewayv1.GRPCRouteSpec{
			Hostnames: []gatewayv1.Hostname{"api.example"},
			Rules: []gatewayv1.GRPCRouteRule{
				{
					BackendRefs: []gatewayv1.GRPCBackendRef{
						{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: "svc-x", Port: portPtr(9090)}}},
					},
				},
			},
		},
	}

	got := convertGRPCRoutes([]gatewayv1.GRPCRoute{gr})

	assert.Equal(t, 1, len(got))
	assert.Equal(t, "api.example", got[0].Spec.Host)
	assert.Equal(t, "/", got[0].Spec.Path)
	assert.Equal(t, "svc-x", got[0].Spec.Service.Name)
	assert.Equal(t, int32(9090), got[0].Spec.Port.TargetPort)
	assert.Equal(t, "grpc-one", got[0].Metadata.Name)
	assert.Equal(t, "ns", got[0].Metadata.Namespace)
}

func updateCacheWithPanic(ctx context.Context, i interface{}) {
	panic("Panic for updating cache")
}

func assertResult(isValid bool, errorMessage string) {
	if !isValid {
		panic(errorMessage)
	}
}
