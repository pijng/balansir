package listenutil

import (
	"balansir/internal/balanceutil"
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/logutil"
	"balansir/internal/metricsutil"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

func tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}
}

//ServeTLSWithAutocert ...
func ServeTLSWithAutocert() {
	configuration := configutil.GetConfig()

	certManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(configuration.AutocertHosts...),
		Cache:      autocert.DirCache(configuration.CertDir),
	}

	go func() {
		metricsutil.MetricsPolling()

		server := &http.Server{
			Addr:         ":" + strconv.Itoa(configuration.Port),
			Handler:      certManager.HTTPHandler(nil),
			ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
		}

		err := server.ListenAndServe()
		if err != nil {
			logutil.Fatal(fmt.Sprintf("Error starting listener: %s", err))
			logutil.Fatal("Shutdown")
			os.Exit(1)
		}
	}()

	TLSConfig := tlsConfig()
	TLSConfig.GetCertificate = certManager.GetCertificate

	TLSServer := &http.Server{
		Addr:         ":" + strconv.Itoa(configuration.TLSPort),
		Handler:      balanceutil.NewServeMux(),
		TLSConfig:    TLSConfig,
		ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
	}

	logutil.Notice("Balansir is up!")
	err := TLSServer.ListenAndServeTLS("", "")
	if err != nil {
		logutil.Fatal(fmt.Sprintf("Error starting TLS listener: %s", err))
		logutil.Fatal("Shutdown")
		os.Exit(1)
	}
}

//ServeTLSWithSelfSignedCerts ...
func ServeTLSWithSelfSignedCerts() {
	configuration := configutil.GetConfig()

	server := &http.Server{
		Addr:         ":" + strconv.Itoa(configuration.TLSPort),
		Handler:      balanceutil.NewServeMux(),
		TLSConfig:    tlsConfig(),
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
		ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
	}

	go func() {
		server := &http.Server{
			Addr:         ":" + strconv.Itoa(configuration.Port),
			Handler:      http.HandlerFunc(helpers.RedirectTLS),
			ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
		}

		err := server.ListenAndServe()
		if err != nil {
			logutil.Fatal(fmt.Sprintf("Error starting listener: %s", err))
			logutil.Fatal("Shutdown")
			os.Exit(1)
		}
	}()

	logutil.Notice("Balansir is up!")
	if err := server.ListenAndServeTLS(configuration.SSLCertificate, configuration.SSLKey); err != nil {
		logutil.Fatal(fmt.Sprintf("Error starting TLS listener: %s", err))
		logutil.Fatal("Shutdown")
		os.Exit(1)
	}
}

//Serve ...
func Serve() {
	configuration := configutil.GetConfig()

	server := http.Server{
		Addr:         ":" + strconv.Itoa(configuration.Port),
		Handler:      balanceutil.NewServeMux(),
		ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
	}

	logutil.Notice("Balansir is up!")
	logutil.Fatal(server.ListenAndServe())
}
