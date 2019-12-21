package confg

import "sync"

//Configuration ...
type Configuration struct {
	Mux                sync.RWMutex
	ServerList         []*Endpoint `json:"server_list"`
	Protocol           string      `json:"connection_protocol"`
	SSLCertificate     string      `json:"ssl_certificate"`
	SSLKey             string      `json:"ssl_private_key"`
	Port               int         `json:"http_port"`
	TLSPort            int         `json:"https_port"`
	Delay              int         `json:"server_check_timer"`
	SessionPersistence bool        `json:"session_persistence"`
	Autocert           bool        `json:"autocert"`
	WhiteHosts         []string    `json:"white_hosts"`
	CertDir            string      `json:"autocert_dir"`
	SessionMaxAge      int         `json:"session_max_age"`
	GzipResponse       bool        `json:"gzip_response"`
	RateLimit          bool        `json:"rate_limit"`
	RatePerSecond      int         `json:"rate_per_second"`
	RateBucket         int         `json:"rate_bucket"`
	Timeout            int         `json:"server_check_response_timeout"`
	ReadTimeout        int         `json:"read_timeout"`
	WriteTimeout       int         `json:"write_timeout"`
	ProxyMode          string      `json:"proxy_mode"`
	Algorithm          string      `json:"balancing_algorithm"`
}

//Endpoint ...
type Endpoint struct {
	URL    string  `json:"endpoint"`
	Weight float64 `json:"weight"`
}
