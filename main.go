package main

import (
	"balansir/internal/configutil"
	"balansir/internal/watchutil"
	"balansir/internal/limitutil"
	"balansir/internal/listenutil"
	"balansir/internal/logutil"
	"balansir/internal/poolutil"
	"balansir/internal/rateutil"
	"fmt"
	"io/ioutil"
	"os"
)

func main() {
	logutil.Init()
	logutil.Info("Booting up...")

	file, err := ioutil.ReadFile("config.yml")
	if err != nil {
		logutil.Fatal(fmt.Sprintf("Error reading configuration file: %v", err))
		logutil.Fatal("Balansir stopped!")
		os.Exit(1)
	}

	if errs := watchutil.FillConfiguration(file); errs != nil {
		logutil.Fatal("Configuration errors:")
		for i := 0; i < len(errs); i++ {
			logutil.Fatal(errs[i])
			if len(errs)-1 == i {
				logutil.Fatal("Balansir stopped!")
				os.Exit(1)
			}
		}
	}

	go poolutil.PoolCheck()
	go watchutil.Watch()

	configuration := configutil.GetConfig()

	if configuration.RateLimit {
		visitors := limitutil.GetLimiter()
		go visitors.CleanOldVisitors()
	}

	rateutil.GetRateCounter()

	if configuration.Protocol == "https" {
		if configuration.Autocert {
			listenutil.ServeTLSWithAutocert()
		} else {
			listenutil.ServeTLSWithSelfSignedCerts()
		}
	} else {
		listenutil.Serve()
	}
}
