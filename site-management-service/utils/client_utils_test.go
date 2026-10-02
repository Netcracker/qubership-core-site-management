package utils

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/security/rest"
	"github.com/netcracker/qubership-core-lib-go/v3/security/tokensource"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

type stubTokenProvider struct {
	security.DummyToken
}

func (p *stubTokenProvider) GetToken(context.Context) (string, error) {
	return "legacy-token", nil
}

type stubTokenSource struct {
	err error
}

func (s *stubTokenSource) GetAudienceToken(context.Context, tokensource.TokenAudience) (string, error) {
	return "k8s-token", s.err
}

func (s *stubTokenSource) GetServiceAccountToken(context.Context) (string, error) {
	return "", nil
}

var k8sTokenSource = &stubTokenSource{}

// useM2MAuthMode makes requests send the tokens of mode; the legacy token is "legacy-token" and the Kubernetes token
// is "k8s-token".
func useM2MAuthMode(t *testing.T, mode security.M2MAuthMode) {
	t.Setenv(security.M2MAuthModeEnv, string(mode))
	getConfig().m2mRequestSender = rest.NewM2MRequestSender()
}

// respondInTurn answers each request with the next status in statuses and records the Authorization header it got.
func respondInTurn(statuses ...int) *[]string {
	var gotAuth []string
	getConfig().do = func(req *fasthttp.Request, resp *fasthttp.Response) error {
		gotAuth = append(gotAuth, string(req.Header.Peek("Authorization")))
		resp.SetStatusCode(statuses[len(gotAuth)-1])
		return nil
	}
	return &gotAuth
}

func TestMain(m *testing.M) {
	serviceloader.Register(1, &security.DummyToken{})
	serviceloader.Register(100, &stubTokenProvider{})
	serviceloader.Register(100, k8sTokenSource)
	os.Exit(m.Run())
}

func TestDoRequestErrOnM2mErr(t *testing.T) {
	useM2MAuthMode(t, security.M2MAuthModeK8s)
	k8sTokenSource.err = errors.New("m2m err")
	t.Cleanup(func() { k8sTokenSource.err = nil })
	resp, err := DoRequest(context.Background(), fasthttp.MethodGet, "http://aaa:8080", nil, logging.GetLogger(""))
	assert.NotNil(t, err)
	assert.Nil(t, resp)
}

func TestConstructRequestFine(t *testing.T) {
	req, err := constructRequest(context.Background(), fasthttp.MethodGet, "http://aaa:8080", nil, "m2m", logging.GetLogger(""))
	assert.Nil(t, err)
	assert.NotNil(t, req)
	assert.Equal(t, "Bearer m2m", string(req.Header.Peek("Authorization")))
	assert.Equal(t, req.Header.Method(), []byte(fasthttp.MethodGet))
	assert.Equal(t, req.RequestURI(), []byte("http://aaa:8080"))
}

func TestDoRetryRequestSecondTryFine(t *testing.T) {
	configloader.Init()
	useM2MAuthMode(t, security.M2MAuthModeLegacy)
	tryNum := 1
	getConfig().do = func(req *fasthttp.Request, resp *fasthttp.Response) error {
		if tryNum == 2 {
			resp.SetStatusCode(fasthttp.StatusOK)
			resp.SetBody([]byte("BodyOK"))
			return nil
		}
		tryNum++
		return errors.New("first error on call")
	}

	resp, err := DoRetryRequest(context.Background(), "", "", nil, logging.GetLogger(""))
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 2, tryNum)
	assert.Equal(t, fasthttp.StatusOK, resp.StatusCode())
	assert.Equal(t, []byte("BodyOK"), resp.Body())
}

func TestDoRequestFine(t *testing.T) {
	useM2MAuthMode(t, security.M2MAuthModeLegacy)
	getConfig().do = func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody([]byte("BodyOK"))
		return nil
	}

	resp, err := DoRequest(context.Background(), "", "", nil, logging.GetLogger(""))
	assert.Nil(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, []byte("BodyOK"), resp.Body())
}

func TestDoRequest_SendsTokenAndBody(t *testing.T) {
	useM2MAuthMode(t, security.M2MAuthModeLegacy)
	var gotAuth, gotBody string
	getConfig().do = func(req *fasthttp.Request, resp *fasthttp.Response) error {
		gotAuth = string(req.Header.Peek("Authorization"))
		gotBody = string(req.Body())
		resp.SetStatusCode(fasthttp.StatusOK)
		return nil
	}

	resp, err := DoRequest(context.Background(), fasthttp.MethodPost, "http://target:8080/api", []byte("payload"), logging.GetLogger(""))

	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, resp.StatusCode())
	assert.Equal(t, "Bearer legacy-token", gotAuth)
	assert.Equal(t, "payload", gotBody)
}

func TestDoRequest_Returns401WithoutResending(t *testing.T) {
	tests := []struct {
		mode     security.M2MAuthMode
		wantAuth string
	}{
		{mode: security.M2MAuthModeLegacy, wantAuth: "Bearer legacy-token"},
		{mode: security.M2MAuthModeK8s, wantAuth: "Bearer k8s-token"},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			useM2MAuthMode(t, tt.mode)
			gotAuth := respondInTurn(fasthttp.StatusUnauthorized)

			resp, err := DoRequest(context.Background(), fasthttp.MethodGet, "http://target:8080/api", nil, logging.GetLogger(""))

			require.NoError(t, err)
			assert.Equal(t, fasthttp.StatusUnauthorized, resp.StatusCode())
			assert.Equal(t, []string{tt.wantAuth}, *gotAuth)
		})
	}
}

func TestDoRequest_HybridResendsWithLegacyTokenAfter401(t *testing.T) {
	useM2MAuthMode(t, security.M2MAuthModeHybrid)
	gotAuth := respondInTurn(fasthttp.StatusUnauthorized, fasthttp.StatusOK, fasthttp.StatusOK)
	var gotBodies []string
	do := getConfig().do
	getConfig().do = func(req *fasthttp.Request, resp *fasthttp.Response) error {
		gotBodies = append(gotBodies, string(req.Body()))
		return do(req, resp)
	}

	resp, err := DoRequest(context.Background(), fasthttp.MethodPost, "http://target:8080/api", []byte("payload"), logging.GetLogger(""))
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, resp.StatusCode())
	_, err = DoRequest(context.Background(), fasthttp.MethodPost, "http://target:8080/api", []byte("payload"), logging.GetLogger(""))

	assert.NoError(t, err)
	assert.Equal(t, []string{"Bearer k8s-token", "Bearer legacy-token", "Bearer legacy-token"}, *gotAuth, "the target keeps the legacy token")
	assert.Equal(t, []string{"payload", "payload", "payload"}, gotBodies)
}

func TestDoRetryRequest_Returns401WithoutResending(t *testing.T) {
	configloader.Init()
	useM2MAuthMode(t, security.M2MAuthModeLegacy)
	gotAuth := respondInTurn(fasthttp.StatusUnauthorized)

	resp, err := DoRetryRequest(context.Background(), fasthttp.MethodGet, "http://target:8080/api", nil, logging.GetLogger(""))

	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusUnauthorized, resp.StatusCode())
	assert.Len(t, *gotAuth, 1)
}

func TestDoRetryRequest_HybridResendsWithLegacyTokenAfter401(t *testing.T) {
	configloader.Init()
	useM2MAuthMode(t, security.M2MAuthModeHybrid)
	gotAuth := respondInTurn(fasthttp.StatusUnauthorized, fasthttp.StatusOK)

	resp, err := DoRetryRequest(context.Background(), fasthttp.MethodGet, "http://target:8080/api", nil, logging.GetLogger(""))

	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, resp.StatusCode())
	assert.Equal(t, []string{"Bearer k8s-token", "Bearer legacy-token"}, *gotAuth)
}

func TestDoRetryRequest_RetriesAfter5xx(t *testing.T) {
	configloader.Init()
	useM2MAuthMode(t, security.M2MAuthModeLegacy)
	var gotBodies []string
	getConfig().do = func(req *fasthttp.Request, resp *fasthttp.Response) error {
		gotBodies = append(gotBodies, string(req.Body()))
		if len(gotBodies) == 1 {
			resp.SetStatusCode(fasthttp.StatusServiceUnavailable)
		} else {
			resp.SetStatusCode(fasthttp.StatusOK)
		}
		return nil
	}

	resp, err := DoRetryRequest(context.Background(), fasthttp.MethodPost, "http://target:8080/api", []byte("payload"), logging.GetLogger(""))

	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, resp.StatusCode())
	assert.Equal(t, []string{"payload", "payload"}, gotBodies)
}

func TestSecureWebSocketDial_HybridRedialsWithLegacyTokenAfter401(t *testing.T) {
	useM2MAuthMode(t, security.M2MAuthModeHybrid)
	var gotAuth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer legacy-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err == nil {
			conn.Close()
		}
	}))
	t.Cleanup(server.Close)
	wsURL, err := url.Parse("ws" + strings.TrimPrefix(server.URL, "http") + "/watch")
	require.NoError(t, err)

	conn, resp, err := SecureWebSocketDial(context.Background(), *wsURL, websocket.Dialer{}, nil, logging.GetLogger(""))

	require.NoError(t, err)
	conn.Close()
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	assert.Equal(t, []string{"Bearer k8s-token", "Bearer legacy-token"}, gotAuth)
}
