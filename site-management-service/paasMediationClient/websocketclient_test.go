package paasMediationClient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
	. "github.com/smarty/assertions"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type fakeWebsocketExecutor struct {
	handler http.Handler
	err     error
}

type badFakeWebsocketExecutor struct {
	attemptCounter int
	handler        http.Handler
	err            error
}

func (*fakeWebsocketExecutor) collectHeaders(ctx context.Context, idpAddress url.URL) (http.Header, error) {
	return http.Header{}, nil
}

func (executor *badFakeWebsocketExecutor) collectHeaders(ctx context.Context, idpAddress url.URL) (http.Header, error) {
	return http.Header{}, nil
}

func (fakeWebsocketExecutor *fakeWebsocketExecutor) createWebsocketConnect(targetAddress url.URL, header http.Header) (*websocket.Conn, *http.Response, error) {
	server := httptest.NewServer(fakeWebsocketExecutor.handler)
	defer server.Close()
	dialer := websocket.Dialer{}
	address := server.Listener.Addr().String
	logger.Info("Address %s", address())
	return dialer.Dial("ws://"+address(), nil)
}

func (executor *badFakeWebsocketExecutor) createWebsocketConnect(targetAddress url.URL, header http.Header) (*websocket.Conn, *http.Response, error) {
	if executor.attemptCounter >= 10 {
		panic("Panic during websocket connect creation due to no more attempts")
	} else {
		executor.attemptCounter++
	}
	return nil, nil, errors.New("Fail websocket connect creation for test")
}

func TestCreateWebSocketClientForRoute(t *testing.T) {
	fakeExecutor := fakeWebsocketExecutor{}
	webSocketClient := WebSocketClient{
		websocketExecutor: &fakeExecutor,
		resource:          routesString,
		namespace:         "test-namespace",
		bus:               make(chan []byte, 50),
	}
	fakeExecutor.handler = http.HandlerFunc(createRouteHandler)
	go webSocketClient.initWebsocketClient(context.Background(), url.URL{Scheme: "ws", Host: "localhost:8080", Path: webSocketClient.generatePath()})
	timer := time.After(3 * time.Second)
	select {
	case <-timer:
		panic("Event on route creation was not gotten")
	case result := <-webSocketClient.bus:
		logger.Info("result %s", result)
		var routeUpdate = new(RouteUpdate)
		if err := json.Unmarshal(result, routeUpdate); err != nil {
			panic(err)
		}
		assertResult(So(routeUpdate.Type, ShouldEqual, updateTypeInit))

		result = <-webSocketClient.bus
		logger.Info("result %s", result)
		routeUpdate = new(RouteUpdate)
		if err := json.Unmarshal(result, routeUpdate); err != nil {
			panic(err)
		}

		assertResult(So(routeUpdate.Type, ShouldEqual, updateTypeAdded))
		assertResult(So(routeUpdate.RouteObject, ShouldResemble, domain.Route{Metadata: domain.Metadata{Name: "test-route", Namespace: "test-namespace"}}))
	}
}

func TestCreateWebSocketClientInitSignal(t *testing.T) {
	fakeExecutor := fakeWebsocketExecutor{}
	webSocketClient := WebSocketClient{
		websocketExecutor: &fakeExecutor,
		resource:          routesString,
		namespace:         "test-namespace",
		bus:               make(chan []byte, 50),
	}
	fakeExecutor.handler = http.HandlerFunc(closeOpenedConnection)
	go webSocketClient.initWebsocketClient(context.Background(), url.URL{Scheme: "ws", Host: "localhost:8080", Path: webSocketClient.generatePath()})
	timer := time.After(3 * time.Second)
	select {
	case <-timer:
		panic("Init signal has not been received")
	case result := <-webSocketClient.bus:
		logger.Info("result %s", result)
		var commonUpdate = new(CommonUpdate)
		if err := json.Unmarshal(result, commonUpdate); err != nil {
			panic(err)
		}
		assertResult(So(commonUpdate.Type, ShouldEqual, updateTypeInit))
		assertResult(So(commonUpdate.CommonObject, ShouldResemble, domain.CommonObject{Metadata: domain.Metadata{Namespace: "test-namespace"}}))
	}
}

func TestReconnectWebSocketClient(t *testing.T) {
	fakeExecutor := badFakeWebsocketExecutor{}
	webSocketClient := WebSocketClient{
		websocketExecutor: &fakeExecutor,
		resource:          routesString,
		namespace:         "test-namespace",
		bus:               make(chan []byte, 50),
	}
	fakeExecutor.handler = http.HandlerFunc(closeOpenedConnection)
	assert.Panics(t, func() {
		webSocketClient.initWebsocketClient(context.Background(), url.URL{Scheme: "ws", Host: "localhost:8080", Path: webSocketClient.generatePath()})
	})
}

func Test_httpRouteAdapter_SplitsIntoMultipleRouteUpdates(t *testing.T) {
	// Build HTTPRoute with two hostnames, one rule with one match and two backends
	r := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-route",
			Namespace: "ns",
			Annotations: map[string]string{
				"x": "y",
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{"h1.example", "h2.example"},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{Path: &gatewayv1.HTTPPathMatch{Value: strPtr("/p")}},
					},
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: "svc-a", Port: portPtr(8080)}}},
						{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: "svc-b"}}}, // nil port -> defaults to 8080 in convertHTTPRoutes
					},
				},
			},
		},
	}

	// Wrap into websocket event payload
	objBytes, err := json.Marshal(r)
	assert.NoError(t, err)
	payload := []byte(fmt.Sprintf(`{"type":"ADDED","object":%s}`, string(objBytes)))

	messages, err := httpRouteAdapter(payload)
	assert.NoError(t, err)
	// 2 hosts x 2 backends x 1 match = 4 updates
	assert.Equal(t, 4, len(messages))

	// Collect and assert each update
	for _, msg := range messages {
		var upd RouteUpdate
		assert.NoError(t, json.Unmarshal(msg, &upd))
		assert.Equal(t, updateTypeAdded, upd.Type)
		assert.Equal(t, "ns", upd.RouteObject.Metadata.Namespace)
		assert.Equal(t, "http-route", upd.RouteObject.Metadata.Name)
		assert.Contains(t, []string{"h1.example", "h2.example"}, upd.RouteObject.Spec.Host)
		assert.Equal(t, "/p", upd.RouteObject.Spec.Path)
		assert.Contains(t, []string{"svc-a", "svc-b"}, upd.RouteObject.Spec.Service.Name)
		if upd.RouteObject.Spec.Service.Name == "svc-a" {
			assert.Equal(t, int32(8080), upd.RouteObject.Spec.Port.TargetPort)
		} else {
			// svc-b defaults to 8080 per convertHTTPRoutes fallback
			assert.Equal(t, int32(8080), upd.RouteObject.Spec.Port.TargetPort)
		}
	}
}

func Test_grpcRouteAdapter_SplitsIntoRouteUpdates(t *testing.T) {
	r := gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "grpc-route", Namespace: "ns"},
		Spec: gatewayv1.GRPCRouteSpec{
			Hostnames: []gatewayv1.Hostname{"api.example"},
			Rules: []gatewayv1.GRPCRouteRule{
				{
					BackendRefs: []gatewayv1.GRPCBackendRef{
						{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: "svc-x", Port: portPtr(9090)}}},
						{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: "svc-y"}}},
					},
				},
			},
		},
	}

	objBytes, err := json.Marshal(r)
	assert.NoError(t, err)
	payload := []byte(fmt.Sprintf(`{"type":"MODIFIED","object":%s}`, string(objBytes)))

	messages, err := grpcRouteAdapter(payload)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(messages))

	for _, msg := range messages {
		var upd RouteUpdate
		assert.NoError(t, json.Unmarshal(msg, &upd))
		assert.Equal(t, updateTypeModified, upd.Type)
		assert.Equal(t, "grpc-route", upd.RouteObject.Metadata.Name)
		assert.Equal(t, "ns", upd.RouteObject.Metadata.Namespace)
		assert.Equal(t, "api.example", upd.RouteObject.Spec.Host)
		assert.Equal(t, "/", upd.RouteObject.Spec.Path)
		if upd.RouteObject.Spec.Service.Name == "svc-x" {
			assert.Equal(t, int32(9090), upd.RouteObject.Spec.Port.TargetPort)
		} else {
			assert.Equal(t, int32(0), upd.RouteObject.Spec.Port.TargetPort)
		}
	}
}

func TestCreateWebSocketClientWithAdapter_DuplicatesMessages(t *testing.T) {
	fakeExecutor := fakeWebsocketExecutor{}
	webSocketClient := WebSocketClient{
		websocketExecutor: &fakeExecutor,
		resource:          httpRoutesString,
		namespace:         "test-namespace",
		adapter: func(b []byte) ([][]byte, error) {
			// Duplicate message to simulate adapter splitting logic
			return [][]byte{b, b}, nil
		},
	}
	fakeExecutor.handler = http.HandlerFunc(createRouteHandler)
	go webSocketClient.initWebsocketClient(context.Background(), url.URL{Scheme: "ws", Host: "localhost:8080", Path: webSocketClient.generatePath()})

	// First message must be init signal
	select {
	case <-time.After(3 * time.Second):
		t.Fatal("init signal not received")
	case result := <-webSocketClient.bus:
		var commonUpdate = new(CommonUpdate)
		if err := json.Unmarshal(result, commonUpdate); err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, updateTypeInit, commonUpdate.Type)
		assert.Equal(t, domain.CommonObject{Metadata: domain.Metadata{Namespace: "test-namespace"}}, commonUpdate.CommonObject)
	}

	// Next two messages should be duplicated by adapter
	for i := 0; i < 2; i++ {
		select {
		case <-time.After(3 * time.Second):
			t.Fatalf("expected adapted message %d not received", i+1)
		case result := <-webSocketClient.bus:
			var routeUpdate = new(RouteUpdate)
			if err := json.Unmarshal(result, routeUpdate); err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, updateTypeAdded, routeUpdate.Type)
			assert.Equal(t, domain.Route{Metadata: domain.Metadata{Name: "test-route", Namespace: "test-namespace"}}, routeUpdate.RouteObject)
		}
	}
}

func createRouteHandler(w http.ResponseWriter, r *http.Request) {
	logger.Info("Get request on websocket connect")
	upgrader := websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	routeUpdate := RouteUpdate{
		Type:        updateTypeAdded,
		RouteObject: domain.Route{Metadata: domain.Metadata{Name: "test-route", Namespace: "test-namespace"}},
	}
	reqBodyBytes := new(bytes.Buffer)
	if err := json.NewEncoder(reqBodyBytes).Encode(routeUpdate); err != nil {
		panic(err)
	}
	reqBodyBytes.Bytes()
	err = c.WriteMessage(websocket.TextMessage, reqBodyBytes.Bytes())
}

func closeOpenedConnection(w http.ResponseWriter, r *http.Request) {
	logger.Info("Get request on websocket connect")
	upgrader := websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c.Close()
}
