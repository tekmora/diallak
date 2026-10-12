package fixture

import (
	"net/http"
	"time"
)

func clients(redirect func(*http.Request, []*http.Request) error) []*http.Client {
	return []*http.Client{
		// ruleid: http-client-must-set-timeout
		&http.Client{},
		// ruleid: http-client-must-set-timeout
		&http.Client{CheckRedirect: redirect},
		// ok: http-client-must-set-timeout
		&http.Client{Timeout: 5 * time.Second},
		// ok: http-client-must-set-timeout
		&http.Client{CheckRedirect: redirect, Timeout: time.Minute},
	}
}
