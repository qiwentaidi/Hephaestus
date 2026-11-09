// Package dockerupdate defines the local protocol between the Web application and its host updater.
package dockerupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	SocketEnv       = "HEPHAESTUS_DOCKER_UPDATER_SOCKET"
	AddressEnv      = "HEPHAESTUS_DOCKER_UPDATER_ADDRESS"
	TokenFile       = "/run/hephaestus-updater/token"
	ImageRepository = "ghcr.io/qiwentaidi/hephaestus"
	VersionLabel    = "org.opencontainers.image.version"

	Pulling    = "pulling"
	Staged     = "staged"
	Restarting = "restarting"
	Ready      = "ready"
	Failed     = "failed"
)

var versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

func ValidVersion(version string) bool { return versionPattern.MatchString(version) }

type Release struct {
	Version     string `json:"version"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Digest      string `json:"digest"`
}

type Status struct {
	Version string `json:"version,omitempty"`
	Phase   string `json:"phase,omitempty"`
	Error   string `json:"error,omitempty"`
}

type Client struct {
	http    *http.Client
	baseURL string
	token   string
}

func ConfiguredClient() *Client {
	if address := os.Getenv(AddressEnv); address != "" {
		token, err := os.ReadFile(TokenFile)
		if err != nil {
			return nil
		}
		return &Client{http: &http.Client{
			Timeout:   25 * time.Second,
			Transport: &http.Transport{DisableKeepAlives: true, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext},
		}, baseURL: "http://" + address, token: strings.TrimSpace(string(token))}
	}
	path := os.Getenv(SocketEnv)
	if path == "" {
		return nil
	}
	return &Client{http: &http.Client{
		Timeout: 25 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", path)
			},
			DisableKeepAlives: true,
		},
	}, baseURL: "http://docker-updater"}
}

func (c *Client) call(ctx context.Context, method, path, version string, result any) error {
	body, err := json.Marshal(Release{Version: version})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return errors.New("Docker 更新器无法连接")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		var failure Status
		if json.NewDecoder(res.Body).Decode(&failure) == nil && failure.Error != "" {
			return errors.New(failure.Error)
		}
		return fmt.Errorf("Docker 更新器请求失败 (%d)", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(result)
}

func (c *Client) Latest(ctx context.Context) (Release, error) {
	var release Release
	err := c.call(ctx, http.MethodGet, "/release", "", &release)
	return release, err
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	var status Status
	err := c.call(ctx, http.MethodGet, "/status", "", &status)
	return status, err
}

func (c *Client) Start(ctx context.Context, operation, version string) (Status, error) {
	var status Status
	err := c.call(ctx, http.MethodPost, "/"+operation, version, &status)
	return status, err
}
