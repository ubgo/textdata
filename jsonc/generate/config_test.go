package generate_test

import "time"

// The representative config struct shared by the golden and round-trip tests.

type Base struct {
	Name string `json:"name" default:"app" doc:"Application name."`
}

type DB struct {
	Host     string `json:"host" default:"localhost" doc:"Database host."`
	Port     int    `json:"port" default:"5432"`
	Password string `json:"password" env:"DB_PASSWORD,required" secret:"true" doc:"Database password."`
}

type TLS struct {
	Cert string `json:"cert" doc:"Certificate file."`
	Key  string `json:"key"`
}

type Server struct {
	Addr        string        `json:"addr" env:"ADDR,notempty" default:":8080" doc:"Listen address."`
	ReadTimeout time.Duration `json:"readTimeout" default:"5s" doc:"How long to wait for a request."`
	TLS         *TLS          `json:"tls"`
}

type Config struct {
	Base
	LogLevel string            `json:"logLevel" enum:"debug,info,warn,error" default:"info" doc:"Log verbosity."`
	Server   Server            `json:"server" doc:"HTTP server."`
	Primary  DB                `json:"primary"`
	Replica  *DB               `json:"replica" doc:"Read replica.\nLeave as is to use the defaults."`
	Hosts    []string          `json:"hosts" default:"a.example, b.example"`
	Limits   map[string]int    `json:"limits" default:"rps:100,burst:20"`
	Ratio    float64           `json:"ratio" default:"0.5"`
	Debug    bool              `json:"debug"`
	Started  time.Time         `json:"started" default:"2026-01-02T03:04:05Z"`
	Workers  *int              `json:"workers" default:"4"`
	ID       int64             `json:"id,string" default:"42"`
	Labels   map[string]string `json:"labels,omitempty"`
	Extra    any               `json:"extra"`
	Skip     string            `json:"-"`
	internal string
}
