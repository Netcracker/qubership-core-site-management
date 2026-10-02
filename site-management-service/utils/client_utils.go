package utils

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/gorilla/websocket"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxhelper"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/security/rest"
	"github.com/valyala/fasthttp"
)

type utilConfig struct {
	m2mRequestSender *rest.M2MRequestSender
	do               func(req *fasthttp.Request, resp *fasthttp.Response) error
	client           *fasthttp.Client
}

var configOnce = sync.Once{}
var config *utilConfig = nil

func createConfig() {
	httpclient := &fasthttp.Client{
		MaxIdleConnDuration:           30 * time.Second,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
		DialDualStack:                 true,
	}
	config = &utilConfig{
		m2mRequestSender: rest.NewM2MRequestSender(),
		do:               httpclient.Do,
		client:           httpclient,
	}
}

func getConfig() *utilConfig {
	if config == nil {
		configOnce.Do(createConfig)
	}
	return config
}

func DoRetryRequest(logContext context.Context, method string, url string, data []byte, logger logging.Logger) (*fasthttp.Response, error) {
	attemptDelayStart, _ := strconv.Atoi(configloader.GetOrDefaultString("http.client.retry.attemptDelay", "2000"))
	attemptDelayStartDuration := time.Duration(attemptDelayStart) * time.Millisecond
	retryLimit, _ := strconv.Atoi(configloader.GetOrDefaultString("http.client.retry.maxAttempts", "5"))

	logger.DebugC(logContext, "Execute secure request (retryLimit: %v, retry delay: %v * n)", retryLimit, attemptDelayStart)
	errMsg := ""
	for i := 0; i < retryLimit; i++ {
		if i > 0 {
			waitInterval := attemptDelayStartDuration * time.Duration(i*i)
			logger.InfoC(logContext, "Sleep %v before retry", waitInterval)
			time.Sleep(waitInterval)
		}

		response, err := DoRequest(logContext, method, url, data, logger)
		if err != nil {
			errMsg = fmt.Sprintf("Retrying request %s %s after error: %s", method, url, err)
			logger.WarnC(logContext, "%s", errMsg)
			continue
		}
		return response, nil
	}
	return nil, errors.New(errMsg)
}

// DoRequest sends the request with the M2M token. In hybrid mode it falls back to the legacy M2M token as
// [rest.M2MRequestSender.Send] does, resending the request after a 401.
func DoRequest(logContext context.Context, method string, url string, data []byte, logger logging.Logger) (*fasthttp.Response, error) {
	var response *fasthttp.Response
	err := getConfig().m2mRequestSender.Send(logContext, url, func(token string) (int, error) {
		if response != nil {
			fasthttp.ReleaseResponse(response)
		}
		var err error
		response, err = doRequestWithToken(logContext, method, url, data, token, logger)
		if err != nil {
			return 0, err
		}
		return response.StatusCode(), nil
	})
	if err != nil {
		if response != nil {
			fasthttp.ReleaseResponse(response)
		}
		return nil, err
	}
	return response, nil
}

func doRequestWithToken(logContext context.Context, method string, url string, data []byte, token string, logger logging.Logger) (*fasthttp.Response, error) {
	req, err := constructRequest(logContext, method, url, data, token, logger)
	defer fasthttp.ReleaseRequest(req)
	if err != nil {
		logger.WarnC(logContext, "Secure %s request handler creation to %s failed with error: %s", method, url, err)
		return nil, err
	}
	response := fasthttp.AcquireResponse()
	err = getConfig().do(req, response)
	if err != nil {
		logger.WarnC(logContext, "Secure %s request to %s failed with error: %s", method, url, err)
		fasthttp.ReleaseResponse(response)
		return nil, err
	}

	if response.StatusCode() >= fasthttp.StatusInternalServerError {
		logger.WarnC(logContext, "Secure %s request to %s failed with 5xx http status code: %d", method, url, response.StatusCode())
		err = errors.New(fmt.Sprintf("Secure %s request to %s failed with 5xx http status code: %v, and body: %s", method, url, response.StatusCode(), string(response.Body())))
		fasthttp.ReleaseResponse(response)
		return nil, err
	}
	return response, nil
}

func constructRequest(ctx context.Context, method string, url string, data []byte, m2mToken string, logger logging.Logger) (*fasthttp.Request, error) {
	req := fasthttp.AcquireRequest()
	logger.DebugC(ctx, "Request will be sent with token")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", m2mToken))
	req.Header.Add("Content-Type", "application/json")

	logger.Debugf(`Building secure request with arguments:
	method=%v, 
	url=%v`, method, url)

	req.Header.SetMethod(method)
	req.SetRequestURI(url)
	req.SetBody(data)

	if err := ctxhelper.AddSerializableContextData(ctx, req.Header.Set); err != nil {
		logger.ErrorC(ctx, "Error during context serializing: %+v", err)
		return req, err
	}

	return req, nil
}

// SecureWebSocketDial dials webSocketURL with the M2M token. In hybrid mode it falls back to the legacy M2M token as
// [rest.M2MRequestSender.Send] does, dialing again after a 401.
func SecureWebSocketDial(logContext context.Context, webSocketURL url.URL, dialer websocket.Dialer, requestHeaders http.Header, logger logging.Logger) (*websocket.Conn, *http.Response, error) {
	if requestHeaders == nil {
		logger.WarnC(logContext, "Headers are nil. Creating default headers")
		requestHeaders = http.Header{}
	}
	requestHeaders = addHeaderIfAbsent(requestHeaders, "Host", webSocketURL.Host)
	var conn *websocket.Conn
	var resp *http.Response
	err := getConfig().m2mRequestSender.Send(logContext, webSocketURL.String(), func(token string) (int, error) {
		headers := addHeaderIfAbsent(requestHeaders.Clone(), "Authorization", "Bearer "+token)
		var err error
		conn, resp, err = dialer.Dial(webSocketURL.String(), headers)
		if resp == nil {
			return 0, err
		}
		return resp.StatusCode, err
	})
	return conn, resp, err
}

func addHeaderIfAbsent(requestHeaders http.Header, headerName, headerValue string) http.Header {
	if _, ok := requestHeaders[headerName]; !ok {
		requestHeaders.Add(headerName, headerValue)
	}
	return requestHeaders
}
