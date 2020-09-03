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
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

func serversCheck() {
	configuration := configutil.GetConfig()
	pool := poolutil.GetPool()
	timer := time.NewTicker(time.Duration(configuration.Delay) * time.Second)
	for {
		select {
		case <-timer.C:
			pool.Guard.Wait()
			inActive := 0
			for _, server := range pool.ServerList {
				active := server.CheckAlive(&configuration.Timeout)
				if !active {
					inActive++
				}
			}
			if inActive == len(pool.ServerList) {
				logutil.Error("All servers are down!")
			}
			configuration.Mux.Lock()
			timer = time.NewTicker(time.Duration(configuration.Delay) * time.Second)
			configuration.Mux.Unlock()
		}
	}
}

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
			pool = poolutil.SetPool(newPool)
		}
	}

	if configuration.Cache {
		args := cacheutil.CacheClusterArgs{
			ShardsAmount:     configuration.CacheShardsAmount,
			MaxSize:          configuration.CacheShardSize,
			ExceedFallback:   configuration.CacheShardExceedFallback,
			CacheAlgorithm:   configuration.CacheAlgorithm,
			BackgroundUpdate: configuration.CacheBackgroundUpdate,
			TransportTimeout: configuration.WriteTimeout,
			DialerTimeout:    configuration.ReadTimeout,
			CacheRules:       configuration.CacheRules,
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
	metricsutil.InitMetricsMeta(rateCounter, configuration, pool.ServerList)
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
				for i := 0; i < len(errs); i++ {
					logutil.Error(fmt.Sprintf("\t %v", errs[i]))
				}
				continue
			}
			logutil.Notice("Configuration changes applied to Balansir")
		}
		time.Sleep(time.Second)
	}
}

var serverPoolHash string
var cacheHash string

func main() {
	logutil.Init()
	logutil.Info("Booting up...")

	file, err := ioutil.ReadFile("config.yml")
	if err != nil {
		logutil.Fatal(fmt.Sprintf("Error reading configuration file: %v", err))
		logutil.Fatal("Shutdown")
		os.Exit(1)
	}

	if errs := fillConfiguration(file); errs != nil {
		logutil.Fatal("Configuration errors:")
		for i := 0; i < len(errs); i++ {
			logutil.Fatal(fmt.Sprintf("\t %v", errs[i]))
			if len(errs)-1 == i {
				logutil.Fatal("Shutdown")
				os.Exit(1)
			}
		}
	}

	go serversCheck()
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
