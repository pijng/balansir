package configutil

import (
	"sync"
)

//Configuration ...
type Configuration struct {
	Mux                      sync.RWMutex
	ServerList               []*Endpoint `json:"server_list"`
	Protocol                 string      `json:"connection_protocol"`
	SSLCertificate           string      `json:"ssl_certificate"`
	SSLKey                   string      `json:"ssl_private_key"`
	Port                     int         `json:"http_port"`
	TLSPort                  int         `json:"https_port"`
	Delay                    int         `json:"server_check_timer"`
	SessionPersistence       bool        `json:"session_persistence"`
	Autocert                 bool        `json:"autocert"`
	AutocertHosts            []string    `json:"autocert_hosts"`
	CertDir                  string      `json:"autocert_dir"`
	SessionMaxAge            int         `json:"session_max_age"`
	GzipResponse             bool        `json:"gzip_response"`
	RateLimit                bool        `json:"rate_limit"`
	RatePerSecond            int         `json:"rate_per_second"`
	RateBucket               int         `json:"rate_bucket"`
	Timeout                  int         `json:"server_check_response_timeout"`
	ReadTimeout              int         `json:"read_timeout"`
	WriteTimeout             int         `json:"write_timeout"`
	TransparentProxyMode     bool        `json:"transparent_proxy_mode"`
	Algorithm                string      `json:"balancing_algorithm"`
	Cache                    bool        `json:"cache"`
	CacheShardsAmount        int         `json:"cache_shards_amount"`
	CacheShardMaxSizeMb      int         `json:"cache_shard_max_size_mb"`
	CacheAlgorithm           string      `json:"cache_algorithm"`
	CacheShardExceedFallback bool        `json:"cache_shard_exceed_fallback"`
	CacheBackgroundUpdate    bool        `json:"cache_background_update"`
	CacheRules               []*Rule     `json:"cache_rules"`
}

//Endpoint ...
type Endpoint struct {
	URL    string  `json:"endpoint"`
	Weight float64 `json:"weight"`
}

//Rule ...
type Rule struct {
	Path string `json:"path"`
	TTL  string `json:"ttl"`
}

var config *Configuration
var once sync.Once

//GetConfig ...
func GetConfig() *Configuration {
	once.Do(func() {
		config = &Configuration{}
	})
	return config
}
