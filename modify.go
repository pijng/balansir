package main

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/gziputil"
	"balansir/internal/helpers"
	"balansir/internal/logutil"
	"bytes"
	"io/ioutil"
	"net/http"
)

//ModifyResponse ...
func ModifyResponse(r *http.Response) error {
	configuration := configutil.GetConfig()

	//Check if response must be gzipped
	if configuration.GzipResponse {
		if gziputil.Allow(r.Header.Get("Content-Type")) {
			r.Body = gziputil.WithGzip(r)
		}
	}

	//Check if URL must be cached
	if ok, TTL := helpers.Contains(r.Request.URL.Path, configuration.CacheRules); ok {
		trackMiss := r.Request.Header.Get("X-Balansir-Background-Update") == ""
		cache := cacheutil.GetCluster()

		//Here we're checking if response' url is not cached.
		_, err := cache.Get(r.Request.URL.Path, trackMiss)
		if err != nil {
			hashedKey := cache.Hash.Sum(r.Request.URL.Path)
			defer cache.Queue.Release(hashedKey)

			//Create bytes buffer for headers and iterate over them
			headerBuf := bytes.NewBuffer([]byte{})

			for key, val := range r.Header {
				//Write header's key to buffer
				headerBuf.Write([]byte(key))
				//Add delimeter so we can split header key later
				headerBuf.Write([]byte(";-;"))
				//Create byte buffer for header value
				headerValueBuf := bytes.NewBuffer([]byte{})
				//Header value is a string slice, so iterate over it to correctly write it to a buffer
				for _, v := range val {
					headerValueBuf.Write([]byte(v))
				}
				//Write complete header value to headers buffer
				headerBuf.Write(headerValueBuf.Bytes())
				//Add another delimeter so we can split headers out of each other
				headerBuf.Write([]byte(";--;"))
			}

			//Read response body, write it to buffer
			b, _ := ioutil.ReadAll(r.Body)
			bodyBuf := bytes.NewBuffer(b)

			//Reassign response body
			r.Body = ioutil.NopCloser(bodyBuf)

			//Create new buffer. Write our headers and body
			respBuf := bytes.NewBuffer([]byte{})
			respBuf.Write(headerBuf.Bytes())
			respBuf.Write(bodyBuf.Bytes())

			err := cache.Set(r.Request.URL.Path, respBuf.Bytes(), TTL)
			if err != nil {
				logutil.Warning(err)
			}
		}
	}
	return nil
}
