// Package clashapi — тонкий клиент к external-controller mihomo (для GUI):
// список серверов группы GEEKCOM-VPN, выбор ноды, пинги, режим.
package clashapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"geekcom-clash/internal/config"
)

const (
	Group    = "GEEKCOM-VPN"
	AutoNode = "GEEKCOM-AUTO"
)

type Client struct {
	base, secret string
	http         *http.Client
}

func New() Client {
	c, _ := config.Load()
	return Client{
		base:   fmt.Sprintf("http://127.0.0.1:%d", c.ControllerPort),
		secret: c.Secret,
		http:   &http.Client{Timeout: 6 * time.Second},
	}
}

func (c Client) do(method, path string, body []byte) ([]byte, error) {
	var r io.Reader
	if body != nil {
		r = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return b, fmt.Errorf("http %d", resp.StatusCode)
	}
	return b, nil
}

type GroupInfo struct {
	Now string   `json:"now"`
	All []string `json:"all"`
}

// Servers — члены GEEKCOM-VPN + текущий выбор (только при запущенном ядре).
func (c Client) Servers() (GroupInfo, error) {
	var g GroupInfo
	b, err := c.do("GET", "/proxies/"+Group, nil)
	if err != nil {
		return g, err
	}
	json.Unmarshal(b, &g)
	return g, nil
}

func (c Client) Select(name string) error {
	_, err := c.do("PUT", "/proxies/"+Group, []byte(fmt.Sprintf(`{"name":%q}`, name)))
	return err
}

func (c Client) Delay(node string) int {
	b, err := c.do("GET", "/proxies/"+url.PathEscape(node)+
		"/delay?url=http://www.gstatic.com/generate_204&timeout=3000", nil)
	if err != nil {
		return 0
	}
	var r struct {
		Delay int `json:"delay"`
	}
	json.Unmarshal(b, &r)
	return r.Delay
}

func (c Client) Mode() string {
	b, err := c.do("GET", "/configs", nil)
	if err != nil {
		return ""
	}
	var r struct {
		Mode string `json:"mode"`
	}
	json.Unmarshal(b, &r)
	return r.Mode
}

// SetMode + фикс GLOBAL: в режиме global направляем встроенный селектор GLOBAL
// на нашу группу (иначе он может висеть на DIRECT).
func (c Client) SetMode(m string) error {
	if _, err := c.do("PATCH", "/configs", []byte(fmt.Sprintf(`{"mode":%q}`, m))); err != nil {
		return err
	}
	if m == "global" {
		c.do("PUT", "/proxies/GLOBAL", []byte(fmt.Sprintf(`{"name":%q}`, Group)))
	}
	return nil
}
