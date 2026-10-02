package paasMediationClient

import (
	"context"
	"net/url"
	"sync"

	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
	"github.com/valyala/fasthttp"
)

type TestHTTPExecutor struct {
	StatusCode int
	StatusByMethod map[string]int
}

func (e *TestHTTPExecutor) doRequest(_ context.Context, url string, method string, _ []byte) (*fasthttp.Response, error) {
	statusCode := e.StatusCode
	if e.StatusByMethod != nil {
		if code, ok := e.StatusByMethod[method+":"+url]; ok {
			statusCode = code
		}
	}
	resp := &fasthttp.Response{}
	resp.SetStatusCode(statusCode)
	return resp, nil
}

func NewTestPaasMediationClientWithCaches(gateway *url.URL, namespace string, executor *TestHTTPExecutor) *PaasMediationClient {
	routesMap := make(map[string]domain.Route)
	servicesMap := make(map[string]domain.Service)
	return &PaasMediationClient{
		Namespace:              namespace,
		InternalGatewayAddress: gateway,
		httpExecutor:           executor,
		cache: &CompositeCache{
			routesCache: &RoutesCache{
				mutex:  &sync.RWMutex{},
				routes: map[string]*map[string]domain.Route{namespace: &routesMap},
			},
			servicesCache: &ServicesCache{
				mutex:    &sync.RWMutex{},
				services: map[string]*map[string]domain.Service{namespace: &servicesMap},
			},
		},
	}
}

func RoutesMap(client *PaasMediationClient, namespace string) map[string]domain.Route {
	return *client.cache.routesCache.routes[namespace]
}

func ServicesMap(client *PaasMediationClient, namespace string) map[string]domain.Service {
	return *client.cache.servicesCache.services[namespace]
}
