//go:build semgrep_fixture

package fixture

import (
	"net/http"
	"time"
)

func send(req *http.Request) (*http.Response, error) {
	// ruleid: no-http-default-client
	return http.DefaultClient.Do(req)
}

func client() *http.Client {
	// ruleid: no-http-default-client
	c := http.DefaultClient
	_ = c
	// ok: no-http-default-client
	return &http.Client{Timeout: time.Second}
}
