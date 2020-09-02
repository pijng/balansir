package configutil

import (
	"sync"
)

//Configuration ...
type Configuration struct {
	Mux                      sync.RWMutex
	ServerList               []*Endpoint `yaml:"server_list"`
	Protocol                 string      `yaml:"connection_protocol"`
	SSLCertificate           string      `yaml:"ssl_certificate"`
	SSLKey                   string      `yaml:"ssl_private_key"`
	Port                     int         `yaml:"http_port"`
	TLSPort                  int         `yaml:"tls_port"`
	Delay                    int         `yaml:"server_check_timer"`
	SessionPersistence       bool        `yaml:"session_persistence"`
	Autocert                 bool        `yaml:"autocert"`
	AutocertHosts            []string    `yaml:"autocert_hosts"`
	CertDir                  string      `yaml:"autocert_dir"`
	SessionMaxAge            int         `yaml:"session_max_age"`
	GzipResponse             bool        `yaml:"gzip_response"`
	RateLimit                bool        `yaml:"rate_limit"`
	RatePerSecond            int         `yaml:"rate_per_second"`
	RateBucket               int         `yaml:"rate_bucket"`
	Timeout                  int         `yaml:"server_check_timeout"`
	ReadTimeout              int         `yaml:"read_timeout"`
	WriteTimeout             int         `yaml:"write_timeout"`
	TransparentProxy         bool        `yaml:"transparent_proxy"`
	Algorithm                string      `yaml:"balancing_algorithm"`
	Cache                    bool        `yaml:"cache"`
	CacheShardsAmount        int         `yaml:"cache_shards_amount"`
	CacheShardSizeMb         int         `yaml:"cache_shard_size_mb"`
	CacheAlgorithm           string      `yaml:"cache_algorithm"`
	CacheShardExceedFallback bool        `yaml:"cache_shard_exceed_fallback"`
	CacheBackgroundUpdate    bool        `yaml:"cache_background_update"`
	CacheRules               []*Rule     `yaml:"cache_rules"`
	ServeStatic              bool        `yaml:"serve_static"`
	StaticFolder             string      `yaml:"static_folder"`
	StaticAlias              string      `yaml:"static_alias"`
}

//Endpoint ...
type Endpoint struct {
	URL    string  `yaml:"endpoint"`
	Weight float64 `yaml:"weight"`
}

//Rule ...
type Rule struct {
	Path string `yaml:"path"`
	TTL  string `yaml:"ttl"`
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
