package paasMediationClient

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	"github.com/netcracker/qubership-core-lib-go/v3/security/rest"
	"github.com/netcracker/qubership-core-site-management/site-management-service/v2/paasMediationClient/domain"
)

var loggerWS logging.Logger

type (
	WebSocketClient struct {
		bus               chan []byte
		namespace         string
		resource          string
		websocketExecutor websocketExecutor
		adapter           Adapter
		wsRetryInterval   time.Duration
	}
	websocketExecutor interface {
		connect(ctx context.Context, targetAddress url.URL) (*websocket.Conn, *http.Response, error)
	}
	defaultWebsocketExecutor struct {
		m2mRequestSender *rest.M2MRequestSender
	}
	Adapter func(t []byte) ([][]byte, error)
)

func init() {
	loggerWS = logging.GetLogger("webSocketClient")
}

func CreateWebSocketClient(ctx context.Context, channel *chan []byte, host, namespace, resource string) {
	CreateWebSocketClientWithAdapter(ctx, channel, host, namespace, resource, nil)
}

func CreateWebSocketClientWithAdapter(ctx context.Context, channel *chan []byte, host, namespace, resource string, adapter Adapter) {
	loggerWS.InfoC(ctx, "Create new web socket client for host '%s', namespace '%s', resource '%s'", host, namespace, resource)
	client := &WebSocketClient{
		bus:               *channel,
		namespace:         namespace,
		resource:          resource,
		websocketExecutor: &defaultWebsocketExecutor{m2mRequestSender: rest.NewM2MRequestSender()},
		adapter:           adapter,
		wsRetryInterval:   configloader.GetKoanf().Duration("paas-mediation.ws-retry-interval"),
	}
	u := url.URL{Scheme: "ws", Host: host, Path: client.generatePath()}

	go client.initWebsocketClient(ctx, u)
}

func (c *WebSocketClient) String() string {
	return fmt.Sprintf("WebSocketClient{namespace=%s,resource=%s,websocketExecutor=%s}", c.namespace, c.resource, c.websocketExecutor)
}

func (c *WebSocketClient) generatePath() string {
	return fmt.Sprintf("/watchapi/v2/paas-mediation/namespaces/%s/%s", c.namespace, c.resource)
}

func (c *WebSocketClient) initWebsocketClient(ctx context.Context, u url.URL) {
	for {
		loggerWS.InfoC(ctx, "Initialize web socket client with address: '%s'", u.String())

		conn, resp, err := c.websocketExecutor.connect(ctx, u)
		if err != nil {
			if resp != nil {
				b, _ := ioutil.ReadAll(resp.Body)
				loggerWS.ErrorC(ctx, "%s", resp.Status)
				loggerWS.ErrorC(ctx, "%s", string(b))
			}
			loggerWS.ErrorC(ctx, "dial error with url '%s': %s", u.String(), err)
			time.Sleep(c.wsRetryInterval)
			continue
		}
		loggerWS.InfoC(ctx, "Status: %s", resp.Status)

		//init signal to get snapshot of resources after connect to websocket
		initSignal, errM := json.Marshal(
			CommonUpdate{
				Type: updateTypeInit,
				CommonObject: domain.CommonObject{
					Metadata: domain.Metadata{Namespace: c.namespace},
				},
			})
		if errM != nil {
			loggerWS.ErrorC(ctx, "Error occurred while marshalling init signal: %s", err.Error())
		} else {
			loggerWS.InfoC(ctx, "Init cache signal: %s", u.String())
			c.bus <- initSignal
		}
		for {
			_, message, err := conn.ReadMessage()
			logger.DebugC(ctx, "Received message by websocket: %s", message)
			if err != nil {
				loggerWS.ErrorC(ctx, "read error from url '%s': %s\nTry to establish web socket connection", u.String(), err)
				conn.Close()
				break
			} else {
				if c.adapter != nil {
					messages, err := c.adapter(message)
					if err != nil {
						loggerWS.ErrorC(ctx, "Error occurred during adapting message: %s", err.Error())
						continue
					}
					for _, message := range messages {
						c.bus <- message
					}
				} else {
					c.bus <- message
				}
			}
		}
	}
}

// connect dials targetAddress with the M2M token. In hybrid mode a 401 to the Kubernetes token makes it dial again
// with the legacy M2M token.
func (websocketExecutor *defaultWebsocketExecutor) connect(ctx context.Context, targetAddress url.URL) (*websocket.Conn, *http.Response, error) {
	dialer := websocket.Dialer{}
	var conn *websocket.Conn
	var resp *http.Response
	err := websocketExecutor.m2mRequestSender.Send(ctx, targetAddress.String(), func(token string) (int, error) {
		header := http.Header{}
		header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
		header.Add("Content-Type", "application/json")
		header.Add("Host", targetAddress.Host)
		header.Add("Origin", "https://"+targetAddress.Host)
		var err error
		conn, resp, err = dialer.Dial(targetAddress.Scheme+"://"+targetAddress.Host+targetAddress.Path, header)
		if resp == nil {
			return 0, err
		}
		return resp.StatusCode, err
	})
	return conn, resp, err
}
