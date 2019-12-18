package helpers

import (
	"balansir/internal/confg"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
)

//RemovePortFromHost ...
func RemovePortFromHost(host string) string {
	if i := strings.Index(host, ":"); i != -1 {
		host = host[:i]
	}
	return host
}

//RedirectTLS ...
func RedirectTLS(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://"+RemovePortFromHost(r.Host), http.StatusMovedPermanently)
}

//RandomStringBytes ...
func RandomStringBytes(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}

//AddRemoteAddrToRequest ...
func AddRemoteAddrToRequest(r *http.Request) *http.Request {
	r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	return r
}

//SetCookieToResponse ...
func SetCookieToResponse(w http.ResponseWriter, hash string, configuration *confg.Configuration) http.ResponseWriter {
	http.SetCookie(w, &http.Cookie{Name: "_balansir_server_hash", Value: hash, MaxAge: configuration.SessionMaxAge})
	return w
}

//ServerPoolsEquals ...
func ServerPoolsEquals(serverPoolHash *string, prevPoolHash string, incomingPool []*confg.Endpoint) bool {
	var sumOfServerHash string
	for _, server := range incomingPool {
		serialized, _ := json.Marshal(server)
		sumOfServerHash += string(serialized)
	}
	md := md5.Sum([]byte(sumOfServerHash))
	poolHash := hex.EncodeToString(md[:16])
	if prevPoolHash == poolHash {
		return false
	}
	serverPoolHash = &poolHash
	return true
}
