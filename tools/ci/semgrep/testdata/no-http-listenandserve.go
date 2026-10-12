//go:build semgrep_fixture

package fixture

import (
	"net/http"
	"time"
)

func serve(h http.Handler) error {
	// ruleid: no-http-listenandserve
	_ = http.ListenAndServe(":8080", h)
	// ruleid: no-http-listenandserve
	_ = http.ListenAndServeTLS(":8443", "cert.pem", "key.pem", h)
	srv := &http.Server{Addr: ":8080", Handler: h, ReadHeaderTimeout: time.Second}
	// ok: no-http-listenandserve
	return srv.ListenAndServe()
}
