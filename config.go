package main

import (
	"encoding/json"
	"flag"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Core link settings
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`

	// Transport
	TLS bool `json:"tls"`

	// Protocol selection: "insp4" or "p10"
	Protocol string `json:"protocol"`

	// Service/server identity
	ServerName string `json:"server_name"` // e.g. services.emechnet.org
	ServerDesc string `json:"server_desc"` // text after SERVER ... :<desc>
	SID        string `json:"sid"`         // e.g. "042" (service/server id for P10/UID prefixes)
	ServiceUID string `json:"service_uid"` // e.g. "042AAAAAA" (UID your service client will use)

	// Q identity (nick introduced by the service)
	QNick string `json:"qnick"`
	QUser string `json:"quser"`
	QHost string `json:"qhost"`
	QReal string `json:"qreal"`

	// Paths (optional)
	AccountsPath string `json:"accounts_path"`
	StatePath    string `json:"state_path"`

	// Logging
	LogLevel string `json:"log_level"`

	// Where to read JSON overrides from (path is parsed here; file is loaded in main.go)
	ConfigFile string `json:"-"`

	// Internal (not serialized)
	ReconnectBackoff time.Duration `json:"-"`
	ReconnectSecs    int           `json:"reconnect_secs"`
}

/***************
 * Loaders
 ***************/

// Note: returns (Config) only, to match your current main.go
func LoadConfigFromFlagsAndEnv() Config {
	var cfg Config

	// Allow passing a config file path which main.go will load & Merge.
	flag.StringVar(&cfg.ConfigFile, "config", getenv("QSERV_CONFIG", ""), "Path to config.json (optional)")

	// Uplink
	host := flag.String("host", getenv("QSERV_HOST", "127.0.0.1"), "Uplink host")
	port := flag.Int("port", getenvInt("QSERV_PORT", 7000), "Uplink port")
	pass := flag.String("pass", getenv("QSERV_PASS", "passwd"), "Uplink password")

	// Transport / protocol
	tlsEnable := flag.Bool("tls", getenvBool("QSERV_TLS", false), "Use TLS to uplink")
	proto := flag.String("proto", getenv("QSERV_PROTOCOL", "insp4"), `Protocol: "insp4" or "p10"`)

	// Identity
	serverName := flag.String("server-name", getenv("QSERV_SERVER_NAME", "services.local"), "Service server name")
	serverDesc := flag.String("server-desc", getenv("QSERV_SERVER_DESC", ""), "Service server description (for SERVER line)")
	sid := flag.String("sid", getenv("QSERV_SID", "042"), "Service/server ID (e.g. 042)")
	svcUID := flag.String("service-uid", getenv("QSERV_SERVICE_UID", ""), "Service UID (optional; defaults from SID)")

	// Q identity
	qnick := flag.String("qnick", getenv("QSERV_QNICK", "Q"), "Q nick")
	quser := flag.String("quser", getenv("QSERV_QUSER", "qserv"), "Q user")
	qhost := flag.String("qhost", getenv("QSERV_QHOST", "services.emechnet.org"), "Q host")
	qreal := flag.String("qreal", getenv("QSERV_QREAL", "The GO Q Bot"), "Q realname")

	// Files
	accPath := flag.String("accounts", getenv("QSERV_ACCOUNTS", "accounts.json"), "Accounts path")
	statePath := flag.String("state", getenv("QSERV_STATE", "state.json"), "State path")

	// Logging / misc
	logLevel := flag.String("log", getenv("QSERV_LOG", "info"), "Log level (debug,info,warn,error)")
	retry := flag.Int("reconnect", getenvInt("QSERV_RECONNECT", 2), "Reconnect seconds")

	flag.Parse()

	// Build the base cfg from flags/env
	cfg.Host = *host
	cfg.Port = *port
	cfg.Password = *pass

	cfg.TLS = *tlsEnable
	cfg.Protocol = *proto

	cfg.ServerName = *serverName
	cfg.ServerDesc = *serverDesc
	cfg.SID = *sid
	cfg.ServiceUID = *svcUID

	cfg.QNick = *qnick
	cfg.QUser = *quser
	cfg.QHost = *qhost
	cfg.QReal = *qreal

	cfg.AccountsPath = *accPath
	cfg.StatePath = *statePath

	cfg.LogLevel = *logLevel
	cfg.ReconnectSecs = *retry

	normalize(&cfg)
	return cfg
}

func LoadConfigFromFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var fc Config
	if err := json.NewDecoder(f).Decode(&fc); err != nil {
		return nil, err
	}
	normalize(&fc)
	return &fc, nil
}

/***************
 * Utilities
 ***************/

// Merge returns a new Config where non-zero values from 'other' override the receiver.
// Note: false for booleans is considered zero and will not override.
func (c Config) Merge(other *Config) Config {
	if other == nil {
		return c
	}
	out := c

	// strings
	mergeStr := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	// ints
	mergeInt := func(dst *int, v int) {
		if v != 0 {
			*dst = v
		}
	}

	mergeStr(&out.Host, other.Host)
	mergeInt(&out.Port, other.Port)
	mergeStr(&out.Password, other.Password)

	// booleans: only override when other.TLS is true (avoids clobbering with zero)
	if other.TLS {
		out.TLS = true
	}

	mergeStr(&out.Protocol, other.Protocol)

	mergeStr(&out.ServerName, other.ServerName)
	mergeStr(&out.ServerDesc, other.ServerDesc)
	mergeStr(&out.SID, other.SID)
	mergeStr(&out.ServiceUID, other.ServiceUID)

	mergeStr(&out.QNick, other.QNick)
	mergeStr(&out.QUser, other.QUser)
	mergeStr(&out.QHost, other.QHost)
	mergeStr(&out.QReal, other.QReal)

	mergeStr(&out.AccountsPath, other.AccountsPath)
	mergeStr(&out.StatePath, other.StatePath)

	mergeStr(&out.LogLevel, other.LogLevel)
	mergeInt(&out.ReconnectSecs, other.ReconnectSecs)

	normalize(&out)
	return out
}

func normalize(c *Config) {
	if c.ReconnectSecs <= 0 {
		c.ReconnectSecs = 2
	}
	c.ReconnectBackoff = time.Duration(c.ReconnectSecs) * time.Second

	if c.Protocol == "" {
		c.Protocol = "insp4"
	}
	if c.SID == "" {
		c.SID = "042"
	}
	if c.ServerName == "" {
		c.ServerName = "services.emechnet.org"
	}
	// If no server-desc provided, fall back to QReal (nice default for banner)
	if c.ServerDesc == "" {
		if c.QReal != "" {
			c.ServerDesc = c.QReal
		} else {
			c.ServerDesc = "EmechNET IRC Services"
		}
	}
	if c.ServiceUID == "" && c.SID != "" {
		// Simple default; your Insp4/P10 code can generate a proper UID if needed.
		c.ServiceUID = c.SID + "AAAAAA"
	}
	if c.QNick == "" {
		c.QNick = "Q"
	}
	if c.QUser == "" {
		c.QUser = "qserv"
	}
	if c.QHost == "" {
		c.QHost = "services.emechnet.org"
	}
	if c.QReal == "" {
		c.QReal = "EmechNET IRC Services"
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getenvInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}

func getenvBool(k string, d bool) bool {
	if v := os.Getenv(k); v != "" {
		switch v {
		case "1", "true", "TRUE", "yes", "YES", "on", "ON":
			return true
		case "0", "false", "FALSE", "no", "NO", "off", "OFF":
			return false
		}
	}
	return d
}
