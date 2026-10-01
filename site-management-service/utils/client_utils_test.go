package utils

import (
	"context"
	"errors"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	serviceloader.Register(1, &security.DummyToken{})
	os.Exit(m.Run())
}

func TestConstructRequestErrOnM2mErr(t *testing.T) {
	getConfig().getToken = func(context.Context) (string, error) {
		return "", errors.New("m2m err")
	}
	req, err := constructRequest(context.Background(), fasthttp.MethodGet, "http://aaa:8080", nil, logging.GetLogger(""))
	assert.NotNil(t, err)
	assert.NotNil(t, req)
}

func TestConstructRequestFine(t *testing.T) {
	getConfig().getToken = func(context.Context) (string, error) {
		return "m2m", nil
	}
	req, err := constructRequest(context.Background(), fasthttp.MethodGet, "http://aaa:8080", nil, logging.GetLogger(""))
	assert.Nil(t, err)
	assert.NotNil(t, req)
	assert.Equal(t, "Bearer m2m", string(req.Header.Peek("Authorization")))
	assert.Equal(t, req.Header.Method(), []byte(fasthttp.MethodGet))
	assert.Equal(t, req.RequestURI(), []byte("http://aaa:8080"))
}

func TestDoRetryRequestSecondTryFine(t *testing.T) {
	configloader.Init()
	getConfig().getToken = func(context.Context) (string, error) {
		return "m2m", nil
	}
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
	getConfig().getToken = func(context.Context) (string, error) {
		return "m2m", nil
	}
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
	getConfig().getToken = func(context.Context) (string, error) {
		return "m2m", nil
	}
	var gotAuth, gotBody string
	getConfig().do = func(req *fasthttp.Request, resp *fasthttp.Response) error {
		gotAuth = string(req.Header.Peek("Authorization"))
		gotBody = string(req.Body())
		resp.SetStatusCode(fasthttp.StatusOK)
		return nil
	}

	resp, err := DoRequest(context.Background(), fasthttp.MethodPost, "http://target:8080/api", []byte("payload"), logging.GetLogger(""))

	assert.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, resp.StatusCode())
	assert.Equal(t, "Bearer m2m", gotAuth)
	assert.Equal(t, "payload", gotBody)
}

func TestDoRequest_Returns401WithoutResending(t *testing.T) {
	getConfig().getToken = func(context.Context) (string, error) {
		return "m2m", nil
	}
	calls := 0
	getConfig().do = func(req *fasthttp.Request, resp *fasthttp.Response) error {
		calls++
		resp.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	}

	resp, err := DoRequest(context.Background(), fasthttp.MethodGet, "http://target:8080/api", nil, logging.GetLogger(""))

	assert.NoError(t, err)
	assert.Equal(t, fasthttp.StatusUnauthorized, resp.StatusCode())
	assert.Equal(t, 1, calls)
}

func TestDoRetryRequest_Returns401WithoutResending(t *testing.T) {
	configloader.Init()
	getConfig().getToken = func(context.Context) (string, error) {
		return "m2m", nil
	}
	calls := 0
	getConfig().do = func(req *fasthttp.Request, resp *fasthttp.Response) error {
		calls++
		resp.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	}

	resp, err := DoRetryRequest(context.Background(), fasthttp.MethodGet, "http://target:8080/api", nil, logging.GetLogger(""))

	assert.NoError(t, err)
	assert.Equal(t, fasthttp.StatusUnauthorized, resp.StatusCode())
	assert.Equal(t, 1, calls)
}

func TestDoRetryRequest_RetriesAfter5xx(t *testing.T) {
	configloader.Init()
	getConfig().getToken = func(context.Context) (string, error) {
		return "m2m", nil
	}
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

	assert.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, resp.StatusCode())
	assert.Equal(t, []string{"payload", "payload"}, gotBodies)
}
