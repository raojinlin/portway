// Package apiclient is a thin HTTP client for the daemon's JSON API, used by
// the CLI subcommands so tunnel state lives in one long-running process
// instead of one process per invocation.
package apiclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"ssh-tunnel-manager/internal/daemon"
)

// Client talks to a running daemon over HTTP.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a client targeting the daemon at addr (host:port, no scheme).
func New(addr string) *Client {
	return &Client{
		baseURL: "http://" + addr,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// ErrDaemonUnreachable is wrapped into errors returned when the daemon
// cannot be reached at all (as opposed to responding with an error status).
var ErrDaemonUnreachable = errors.New("daemon unreachable")

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		// http.Client.Do only returns an error for transport-level failures
		// (dial refused, timeout, DNS, ...); HTTP error statuses come back
		// as a normal response with err == nil and are handled below.
		return fmt.Errorf("%w: %s (%v)", ErrDaemonUnreachable, c.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("daemon returned %s", resp.Status)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) List() ([]daemon.TunnelView, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/tunnels", nil)
	if err != nil {
		return nil, err
	}
	var views []daemon.TunnelView
	if err := c.do(req, &views); err != nil {
		return nil, err
	}
	return views, nil
}

func (c *Client) Get(name string) (daemon.TunnelView, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/tunnels/"+name, nil)
	if err != nil {
		return daemon.TunnelView{}, err
	}
	var v daemon.TunnelView
	if err := c.do(req, &v); err != nil {
		return daemon.TunnelView{}, err
	}
	return v, nil
}

func (c *Client) Create(tr daemon.TunnelRequest) (daemon.TunnelView, error) {
	body, err := json.Marshal(tr)
	if err != nil {
		return daemon.TunnelView{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/tunnels", bytes.NewReader(body))
	if err != nil {
		return daemon.TunnelView{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	var v daemon.TunnelView
	if err := c.do(req, &v); err != nil {
		return daemon.TunnelView{}, err
	}
	return v, nil
}

func (c *Client) Update(name string, tr daemon.TunnelRequest) (daemon.TunnelView, error) {
	body, err := json.Marshal(tr)
	if err != nil {
		return daemon.TunnelView{}, err
	}
	req, err := http.NewRequest(http.MethodPut, c.baseURL+"/api/tunnels/"+name, bytes.NewReader(body))
	if err != nil {
		return daemon.TunnelView{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	var v daemon.TunnelView
	if err := c.do(req, &v); err != nil {
		return daemon.TunnelView{}, err
	}
	return v, nil
}

func (c *Client) Delete(name string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/api/tunnels/"+name, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) Start(name string) (daemon.TunnelView, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/tunnels/"+name+"/start", nil)
	if err != nil {
		return daemon.TunnelView{}, err
	}
	var v daemon.TunnelView
	if err := c.do(req, &v); err != nil {
		return daemon.TunnelView{}, err
	}
	return v, nil
}

func (c *Client) Stop(name string) (daemon.TunnelView, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/tunnels/"+name+"/stop", nil)
	if err != nil {
		return daemon.TunnelView{}, err
	}
	var v daemon.TunnelView
	if err := c.do(req, &v); err != nil {
		return daemon.TunnelView{}, err
	}
	return v, nil
}
