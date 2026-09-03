package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// This file re-implements (rather than imports, to avoid engine depending on
// the separate kue module) the small slice of kue/modules/shared's config +
// authenticated-request logic needed to push/read WSL run history: reading
// ~/.kue/config.json (or $KUE_CONFIG_PATH) written by `kue login`, and
// issuing Bearer-token JSON requests against the same kuetix API it talks
// to. Keep this in sync with kue/modules/shared/helpers.go if that format
// changes.

const defaultAPIHost = "api.kuetix.com"

type kueConfig struct {
	Host  string                 `json:"host,omitempty"`
	Login map[string]interface{} `json:"login,omitempty"`
}

func defaultKueConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kue", "config.json")
}

func loadKueConfigFile() (kueConfig, error) {
	var cfg kueConfig
	path := strings.TrimSpace(os.Getenv("KUE_CONFIG_PATH"))
	if path == "" {
		path = defaultKueConfigPath()
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// loginToken mirrors kue/modules/shared.GetLoginToken: the token may be a
// direct key on `login`, or nested one level under `login.data`.
func (c kueConfig) loginToken() string {
	if c.Login == nil {
		return ""
	}
	for _, key := range []string{"token", "jwt", "access_token"} {
		if v, ok := c.Login[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	if data, ok := c.Login["data"].(map[string]interface{}); ok {
		for _, key := range []string{"token", "jwt", "access_token"} {
			if v, ok := data[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "https://" + defaultAPIHost
	}
	if strings.HasPrefix(strings.ToLower(host), "https://") {
		return "https://" + strings.TrimPrefix(host, "https://")
	}
	return "http://" + strings.TrimPrefix(host, "http://")
}

func resolveAPIHost(cfg kueConfig) string {
	if envHost := strings.TrimSpace(os.Getenv("KUE_HOST")); envHost != "" {
		return normalizeHost(envHost)
	}
	if strings.TrimSpace(cfg.Host) != "" {
		return normalizeHost(cfg.Host)
	}
	return normalizeHost(defaultAPIHost)
}

// apiClient issues authenticated JSON requests against the kuetix API using
// the credentials `kue login` already stored on disk.
type apiClient struct {
	host  string
	token string
}

func newAPIClient() (*apiClient, error) {
	cfg, err := loadKueConfigFile()
	if err != nil {
		return nil, fmt.Errorf("read kue config: %w", err)
	}
	token := cfg.loginToken()
	if token == "" {
		return nil, fmt.Errorf("no kuetix API token found; run `kue login` first")
	}
	return &apiClient{host: resolveAPIHost(cfg), token: token}, nil
}

func (c *apiClient) do(method, path string, payload interface{}) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimRight(c.host, "/")+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func (c *apiClient) post(path string, payload interface{}) (int, []byte, error) {
	return c.do(http.MethodPost, path, payload)
}

func (c *apiClient) get(path string) (int, []byte, error) {
	return c.do(http.MethodGet, path, nil)
}
