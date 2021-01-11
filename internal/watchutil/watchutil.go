package watchutil

import (
	"balansir/internal/cacheutil"
	"balansir/internal/configutil"
	"balansir/internal/helpers"
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
	"time"

	"gopkg.in/yaml.v2"
)

var serverPoolHash string
var cacheHash string

//FillConfiguration ...
func FillConfiguration(file []byte) []error {
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

//Watch ...
func Watch() {
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
			errs := FillConfiguration(file)
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
