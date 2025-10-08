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
)

type (
	fakeHttpExecutor struct {
		response                          *fasthttp.Response
		isCreateSecureRequestMethodCalled bool
		requestUrl                        string
		err                               error
		doRequestFunc                     func(ctx context.Context, url, method string, body []byte) (*fasthttp.Response, error)
	}
)

func (fakeHttpExecutor *fakeHttpExecutor) doRequest(ctx context.Context, url string, method string, body []byte) (*fasthttp.Response, error) {
	fakeHttpExecutor.isCreateSecureRequestMethodCalled = true
	fakeHttpExecutor.requestUrl = url
	if fakeHttpExecutor.doRequestFunc != nil {
		return fakeHttpExecutor.doRequestFunc(ctx, url, method, body)
	}
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

func updateCacheWithPanic(ctx context.Context, i interface{}) {
	panic("Panic for updating cache")
}

func assertResult(isValid bool, errorMessage string) {
	if !isValid {
		panic(errorMessage)
	}
}

func newFakeHttpExecutorWithFailures(failCount int, successBody interface{}, successCode int) *fakeHttpExecutor {
	counter := 0
	return &fakeHttpExecutor{
		doRequestFunc: func(_ context.Context, _ string, _ string, _ []byte) (*fasthttp.Response, error) {
			if counter < failCount {
				counter++
				resp := fasthttp.AcquireResponse()
				resp.SetStatusCode(500)
				return resp, nil
			}
			// success
			resp := fasthttp.AcquireResponse()
			bodyBytes, _ := json.Marshal(successBody)
			resp.SetBody(bodyBytes)
			resp.SetStatusCode(successCode)
			return resp, nil
		},
	}
}

func TestCreateAndDeleteService(t *testing.T) {
	// Prepare a service object
	svc := domain.Service{
		Metadata: domain.Metadata{Name: "svc1", Namespace: "ns1"},
	}
	// Executor returns the svc object on create, HTTP 201
	execCreate := newFakeHttpExecutor(&svc, 201)
	gw, _ := url.Parse("http://gateway")
	client := &PaasMediationClient{
		InternalGatewayAddress: gw,
		httpExecutor:           execCreate,
	}
	// initialize service cache map
	client.cache = &CompositeCache{
		servicesCache: &ServicesCache{
			mutex:    &sync.RWMutex{},
			services: map[string]*map[string]domain.Service{"ns1": {}},
		},
	}

	err := client.CreateService(context.Background(), &svc, "ns1")
	assert.NoError(t, err)
	// Should have been called
	assert.True(t, execCreate.isCreateSecureRequestMethodCalled)
	// Check it is in cache
	svcMap := *client.cache.servicesCache.services["ns1"]
	assert.Contains(t, svcMap, "svc1")

	// Now test DeleteService
	execDel := newFakeHttpExecutor(nil, 200)
	client.httpExecutor = execDel
	err = client.DeleteService(context.Background(), "svc1", "ns1")
	assert.NoError(t, err)
	// Should be removed
	svcMap2 := *client.cache.servicesCache.services["ns1"]
	assert.NotContains(t, svcMap2, "svc1")
}

func TestPerformRequestWithRetry_EventualSuccess(t *testing.T) {
	// simulate 2 failures then success
	exec := newFakeHttpExecutorWithFailures(2, []domain.Route{{
		Metadata: domain.Metadata{
			Name:      "test-route",
			Namespace: "test-namespace",
		},
		Spec: domain.RouteSpec{
			Host: "local",
			Path: "path",
		},
	}}, 200)
	gw, _ := url.Parse("http://gateway")
	client := &PaasMediationClient{
		InternalGatewayAddress: gw,
		httpExecutor:           exec,
	}

	urlStr := "http://gateway/api/v2/paas-mediation/namespaces/ns1/routes"
	var out []domain.Route
	err := client.performRequestWithRetry(context.Background(), urlStr, "GET", nil, 200, &out)
	assert.NoError(t, err)
}

func TestSyncingCache_InvalidJSON(t *testing.T) {
	rc := &RoutesCache{
		mutex: &sync.RWMutex{},
		routes: map[string]*map[string]domain.Route{
			"ns1": {},
		},
		bus: make(chan []byte, 1),
	}
	comp := &CompositeCache{routesCache: rc}
	client := &PaasMediationClient{cache: comp}

	cb := CommonUpdateStr{
		updateCache: client.cache.updateRoutesCache,
		resource:    RouteUpdate{},
		bus:         rc.bus,
	}
	// run syncing loop with tiny sleep
	go syncingCacheInternal(context.Background(), cb, time.Millisecond)
	// send invalid JSON
	rc.bus <- []byte("invalid-json-%%%")
	time.Sleep(10 * time.Millisecond)
	// No panic, and still empty map
	assert.Empty(t, *client.cache.routesCache.routes["ns1"])
}

func TestCreateUpdateDeleteRoute(t *testing.T) {
	route := domain.Route{
		Metadata: domain.Metadata{Name: "routeX", Namespace: "ns2"},
	}
	exec := newFakeHttpExecutor(&route, 201)
	gw, _ := url.Parse("http://gateway")
	client := &PaasMediationClient{
		InternalGatewayAddress: gw,
		httpExecutor:           exec,
	}
	// initialize route cache
	client.cache = &CompositeCache{
		routesCache: &RoutesCache{
			mutex:  &sync.RWMutex{},
			routes: map[string]*map[string]domain.Route{"ns2": {}},
		},
	}

	// Create
	err := client.CreateRoute(context.Background(), &route, "ns2")
	assert.NoError(t, err)
	assert.Contains(t, *client.cache.routesCache.routes["ns2"], "routeX")

	// UpdateOrCreate (modify path)
	route.Spec.Host = "newhost.example.com"
	exec2 := newFakeHttpExecutor(&route, 200)
	client.httpExecutor = exec2
	err = client.UpdateOrCreateRoute(context.Background(), &route, "ns2")
	assert.NoError(t, err)
	// Should update in cache
	updatedRoute := (*client.cache.routesCache.routes["ns2"])["routeX"]
	assert.Equal(t, "newhost.example.com", updatedRoute.Spec.Host)

	// Delete
	execDel := newFakeHttpExecutor(nil, 200)
	client.httpExecutor = execDel
	err = client.DeleteRoute(context.Background(), "ns2", "routeX")
	assert.NoError(t, err)
	assert.NotContains(t, *client.cache.routesCache.routes["ns2"], "routeX")
}

func TestGetRoutesSuite(t *testing.T) {
	gw, _ := url.Parse("http://gateway")

	client := &PaasMediationClient{
		InternalGatewayAddress: gw,
		Namespace:              "nsX",
	}
	client.cache = &CompositeCache{
		routesCache: &RoutesCache{
			mutex:  &sync.RWMutex{},
			routes: map[string]*map[string]domain.Route{},
			bus:    make(chan []byte, 1),
			initCache: func(ctx context.Context, namespace string) {
				// no-op for test
			},
		},
	}

	// Prepopulate cache for multiple namespaces
	ns1Routes := map[string]domain.Route{
		"r1": {Metadata: domain.Metadata{Name: "r1", Namespace: "ns1"}},
		"r2": {Metadata: domain.Metadata{Name: "r2", Namespace: "ns1"}},
	}
	ns2Routes := map[string]domain.Route{
		"r3": {Metadata: domain.Metadata{Name: "r3", Namespace: "ns2"}},
	}
	client.cache.routesCache.routes["ns1"] = &ns1Routes
	client.cache.routesCache.routes["ns2"] = &ns2Routes

	t.Run("GetRoutes returns all routes in namespace", func(t *testing.T) {
		routes, err := client.GetRoutes(context.Background(), "ns1")
		assert.NoError(t, err)
		assert.Len(t, *routes, 2)
	})

	t.Run("GetRoutesByFilter returns filtered routes", func(t *testing.T) {
		filtered, err := client.GetRoutesByFilter(context.Background(), "ns1", func(r *domain.Route) bool {
			return r.Metadata.Name == "r1"
		})
		assert.NoError(t, err)
		assert.Len(t, *filtered, 1)
		assert.Equal(t, "r1", (*filtered)[0].Metadata.Name)
	})

	t.Run("GetRoutesForNamespaces returns combined routes", func(t *testing.T) {
		routes, err := client.GetRoutesForNamespaces(context.Background(), []string{"ns1", "ns2"})
		assert.NoError(t, err)
		assert.Len(t, *routes, 3)

		names := []string{(*routes)[0].Metadata.Name, (*routes)[1].Metadata.Name, (*routes)[2].Metadata.Name}
		assert.ElementsMatch(t, names, []string{"r1", "r2", "r3"})
	})

	t.Run("GetRoutesForNamespaces2 returns filtered routes across namespaces", func(t *testing.T) {
		filtered, err := client.GetRoutesForNamespaces2(context.Background(), []string{"ns1", "ns2"}, func(r *domain.Route) bool {
			return r.Metadata.Name == "r3"
		})
		assert.NoError(t, err)
		assert.Len(t, *filtered, 1)
		assert.Equal(t, "r3", (*filtered)[0].Metadata.Name)
	})
}

func TestUpdateOrCreateService(t *testing.T) {
	ctx := context.Background()
	gw, _ := url.Parse("http://gateway")

	// Create a new client with empty cache
	client := &PaasMediationClient{
		InternalGatewayAddress: gw,
		Namespace:              "nsX",
	}

	// Initialize composite cache
	client.cache = &CompositeCache{
		servicesCache: &ServicesCache{
			mutex: &sync.RWMutex{},
			services: map[string]*map[string]domain.Service{
				"nsX": {},
			},
			bus:       make(chan []byte, 1),
			initCache: client.InitServicesMapInCache,
		},
	}

	// Service to update/create
	svc := &domain.Service{
		Metadata: domain.Metadata{Name: "svc1", Namespace: "nsX"},
	}

	// Fake HTTP executor returning 200 OK for PUT requests
	client.httpExecutor = &httpExecutorImpl{}
	client.httpExecutor = &fakeHttpExecutor{
		doRequestFunc: func(_ context.Context, url, method string, body []byte) (*fasthttp.Response, error) {
			resp := fasthttp.AcquireResponse()
			resp.SetStatusCode(fasthttp.StatusOK)
			return resp, nil
		},
	}

	// Call UpdateOrCreateService
	err := client.UpdateOrCreateService(ctx, svc, "nsX")
	assert.NoError(t, err)

	// Check if service was added to cache
	client.cache.servicesCache.mutex.RLock()
	cachedService, ok := (*client.cache.servicesCache.services["nsX"])["svc1"]
	client.cache.servicesCache.mutex.RUnlock()

	assert.True(t, ok, "Service should be in cache")
	assert.Equal(t, "svc1", cachedService.Metadata.Name)
}
