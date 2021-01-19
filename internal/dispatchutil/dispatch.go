package dispatchutil

import (
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/rateutil"
	"balansir/internal/serverutil"
	"net/http"
	"net/http/httptrace"
	"time"
)

func setSecureHeaders(w http.ResponseWriter) http.ResponseWriter {
	w.Header().Add("X-XSS-Protection", "1; mode=block")
	w.Header().Add("X-Content-Type-Options", "nosniff")
	w.Header().Add("X-Frame-Options", "deny")
	return w
}

//Dispatch ...
func Dispatch(endpoint *serverutil.Server, w http.ResponseWriter, r *http.Request) {
	configuration := configutil.GetConfig()
	rateCounter := rateutil.GetRateCounter()

	trackResponseTime := r.Header.Get("X-Balansir-Background-Update") == ""
	var requestStart time.Time

	trace := &httptrace.ClientTrace{
		GotConn: func(httptrace.GotConnInfo) {
			endpoint.IncreaseActiveConnections()
			if trackResponseTime {
				requestStart = time.Now()
				rateCounter.HitRequest()
			}
		},
		GotFirstResponseByte: func() {
			endpoint.DecreaseActiveConnections()
			if trackResponseTime {
				rateCounter.CommitResponseTime(requestStart)
			}
		},
	}

	r = r.WithContext(httptrace.WithClientTrace(r.Context(), trace))
	w = setSecureHeaders(w)

	if configuration.SessionPersistence {
		w = helpers.SetSession(w, endpoint.ServerHash, configuration.SessionMaxAge)
	}

	endpoint.Proxy.ServeHTTP(w, r)
}
