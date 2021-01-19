package proxyutil

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/gziputil"
	"balansir/internal/logutil"
	"balansir/internal/statusutil"
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

//ModifyResponse ...
func ModifyResponse(r *http.Response) error {
	//TODO move this to httptrace.ClientTrace in dispatchutil
	statusCodes := statusutil.GetStatusCodes()
	statusCodes.HitStatus(r.StatusCode)

	configuration := configutil.GetConfig()

	//Check if response must be gzipped
	if configuration.GzipResponse {
		if gziputil.Allow(r.Header.Get("Content-Type")) {
			r.Body = gziputil.WithGzip(r)
		}
	}

	if !configuration.Cache.Enabled && cacheutil.GetCluster() == nil {
		return nil
	}

	mustBeCached, TTL := cacheutil.ContainsRule(r.Request.URL.Path, configuration.Cache.Rules)
	if !mustBeCached {
		return nil
	}

	trackMiss := r.Request.Header.Get("X-Balansir-Background-Update") == ""
	cache := cacheutil.GetCluster()

	_, err := cache.Get(r.Request.URL.Path, trackMiss)
	//err == nil means that response for a given key is already cached
	if err == nil {
		return nil
	}

	hashedKey := cache.Hash.Sum(r.Request.URL.Path)
	defer cache.Queue.Release(hashedKey)

	headersBuf := bytes.NewBuffer([]byte{})

	for key, val := range r.Header {
		headersBuf.Write([]byte(key))
		//Add delimeter so we can split header's key and value later on
		headersBuf.Write(cacheutil.KeyValueDelimeter)

		headerValueBuf := bytes.NewBuffer([]byte{})
		//Header value is a string slice
		headerValueBuf.Write([]byte(strings.Join(val, "")))

		headersBuf.Write(headerValueBuf.Bytes())
		//Add delimeter so we can split pairs out of each other later on
		headersBuf.Write(cacheutil.PairDelimeter)
	}

	//Add delimeter so we can split headers from body later on
	headersBuf.Write(cacheutil.HeadersDelimeter)

	b, _ := ioutil.ReadAll(r.Body)
	bodyBuf := bytes.NewBuffer(b)

	//Reassign and close response body with no-op
	r.Body = ioutil.NopCloser(bodyBuf)

	responseBuf := bytes.NewBuffer([]byte{})
	responseBuf.Write(headersBuf.Bytes())
	responseBuf.Write(bodyBuf.Bytes())

	err = cache.Set(r.Request.URL.Path, responseBuf.Bytes(), TTL)
	if err != nil {
		logutil.Warning(err)
	}

	return nil
}

//ErrorHandler ...
func ErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		// Suppress `context canceled` error.
		// It may occur when client cancels the request with fast refresh
		// or by closing the connection. This error isn't informative at all and it'll
		// just junk the log around.
		if err.Error() == "context canceled" {
		} else {
			logutil.Error(fmt.Sprintf(`proxy error: %s`, err.Error()))
		}
	}
}
