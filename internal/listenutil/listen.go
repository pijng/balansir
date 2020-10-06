package listenutil

import (
	"balansir/internal/balanceutil"
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/logutil"
	"balansir/internal/metricsutil"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

const (
	autocertDir = "./certs"
)

func tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
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

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	certManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(configuration.AutocertHosts...),
		Cache:      autocert.DirCache(autocertDir),
	}

	go func() {
		metricsutil.MetricsPolling()

		server := &http.Server{
			Addr:         ":" + strconv.Itoa(configuration.Port),
			Handler:      certManager.HTTPHandler(nil),
			ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
		}

		logutil.Fatal(server.ListenAndServe())

		<-done
		gracefulShutdown(done, server)
	}()

	TLSConfig := tlsConfig()
	TLSConfig.GetCertificate = certManager.GetCertificate

	TLSServer := &http.Server{
		Addr:         ":" + strconv.Itoa(configuration.TLSPort),
		Handler:      balanceutil.NewServeMux(),
		TLSConfig:    TLSConfig,
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
	}

	go func() {
		logutil.Fatal(TLSServer.ListenAndServeTLS("", ""))
	}()
	logutil.Notice("Balansir is up!")

	<-done
	gracefulShutdown(done, TLSServer)
}

//ServeTLSWithSelfSignedCerts ...
func ServeTLSWithSelfSignedCerts() {
	configuration := configutil.GetConfig()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		server := &http.Server{
			Addr:         ":" + strconv.Itoa(configuration.Port),
			Handler:      http.HandlerFunc(helpers.RedirectTLS),
			ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
		}

		logutil.Fatal(server.ListenAndServe())

		<-done
		gracefulShutdown(done, server)
	}()

	TLSServer := &http.Server{
		Addr:         ":" + strconv.Itoa(configuration.TLSPort),
		Handler:      balanceutil.NewServeMux(),
		TLSConfig:    tlsConfig(),
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
	}

	go func() {
		logutil.Fatal(TLSServer.ListenAndServeTLS(configuration.SSLCertificate, configuration.SSLKey))
	}()
	logutil.Notice("Balansir is up!")

	<-done
	gracefulShutdown(done, TLSServer)
}

//Serve ...
func Serve() {
	configuration := configutil.GetConfig()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	server := &http.Server{
		Addr:         ":" + strconv.Itoa(configuration.Port),
		Handler:      balanceutil.NewServeMux(),
		ReadTimeout:  time.Duration(configuration.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(configuration.WriteTimeout) * time.Second,
	}

	go func() {
		logutil.Fatal(server.ListenAndServe())
	}()
	logutil.Notice("Balansir is up!")

	<-done
	gracefulShutdown(done, server)
}

func gracefulShutdown(signal chan os.Signal, server *http.Server) {
	logutil.Notice("Shutting down Balansir...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err := server.Shutdown(ctx); err != nil {
		logutil.Fatal(fmt.Sprintf("Balansir shutdown failed: %+v", err))
	}
	logutil.Notice("Balansir stopped!")
}
