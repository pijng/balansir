package main

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/helpers"
	"balansir/internal/limitutil"
	"balansir/internal/listenutil"
	"balansir/internal/logutil"
	"balansir/internal/metricsutil"
	"balansir/internal/poolutil"
	"balansir/internal/rateutil"
	"balansir/internal/statusutil"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

var serverPoolHash string
var cacheHash string

func fillConfiguration(file []byte) []error {
	configuration := configutil.GetConfig()
	configuration.Guard.Add(1)
	defer configuration.Guard.Done()

	configuration.Mux.Lock()
	defer configuration.Mux.Unlock()

	pool := poolutil.GetPool()
	pool.Guard.Add(1)
	defer pool.Guard.Done()

	var errs []error
	if err := yaml.Unmarshal(file, &configuration); err != nil {
		errs = append(errs, errors.New(fmt.Sprint("config.yml malformed: ", err)))
		return errs
	}

	if !helpers.ServerPoolsEquals(&serverPoolHash, configuration.ServerList) {
		newPool, err := poolutil.RedefineServerPool(configuration, &pool.Guard)
		if err != nil {
			errs = append(errs, err)
		}
		if newPool != nil {
			poolutil.SetPool(newPool)
		}
	}

	if configuration.Cache.Enabled {
		args := cacheutil.CacheClusterArgs{
			ShardsAmount:     configuration.Cache.ShardsAmount,
			ShardSize:        configuration.Cache.ShardSize,
			ExceedFallback:   configuration.Cache.ShardExceedFallback,
			CacheAlgorithm:   configuration.Cache.Algorithm,
			BackgroundUpdate: configuration.Cache.BackgroundUpdate,
			CacheRules:       configuration.Cache.Rules,
			TransportTimeout: configuration.WriteTimeout,
			DialerTimeout:    configuration.ReadTimeout,
			Port:             configuration.Port,
		}

		if !cacheutil.CacheEquals(&cacheHash, &args) {
			err := cacheutil.RedefineCache(&args)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	rateCounter := rateutil.GetRateCounter()
	statusCodes := statusutil.GetStatusCodes()
	metricsutil.InitMetricsMeta(rateCounter, configuration, pool.ServerList, statusCodes)
	return errs
}

func configWatch() {
	file, err := ioutil.ReadFile("config.yml")
	if err != nil {
		logutil.Error(fmt.Sprintf("Error reading configuration file: %v", err))
	}

	md := md5.Sum(file)
	fileHash := hex.EncodeToString(md[:16])
	var fileHashNext string

	for {
		file, _ = ioutil.ReadFile("config.yml")
		md = md5.Sum(file)
		fileHashNext = hex.EncodeToString(md[:16])

		if fileHash != fileHashNext {
			fileHash = fileHashNext
			errs := fillConfiguration(file)
			if errs != nil {
				logutil.Error("Configuration errors:")
				for _, err := range errs {
					logutil.Error(err)
				}
				continue
			}
			logutil.Notice("Configuration changes applied to Balansir")
		}

		time.Sleep(time.Second)
	}
}

func main() {
	logutil.Init()
	logutil.Info("Booting up...")

	file, err := ioutil.ReadFile("config.yml")
	if err != nil {
		logutil.Fatal(fmt.Sprintf("Error reading configuration file: %v", err))
		logutil.Fatal("Balansir stopped!")
		os.Exit(1)
	}

	if errs := fillConfiguration(file); errs != nil {
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
	go configWatch()

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
