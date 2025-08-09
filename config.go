package main

import (
	"flag"
	"fmt"
)

// Config holds all runtime settings parsed from flags.
type Config struct {
	// Uplink (remote server we connect to)
	Host string
	Port int

	// Our server identity (what we present to the uplink)
	ServerName string // a.k.a. -server / -name
	ServerDesc string
	Password   string
	SID        string // 3-char server SID

	// Protocol + logging
	Proto    string // e.g. "insp4"
	LogLevel string // debug|info|warn|error

	// Service client (Q) identity
	QNick string // -qnick
	QHost string // -qhost
	QUser string // -quser (ident)
	QReal string // -qreal (realname/gecos)

	// Service behavior
	AutoJoin     string // -chan (default #feds)
	AccountsFile string // -accounts (default accounts.json)

	// Derived at runtime
	ServiceUID string
}

// ParseConfig defines CLI flags, parses them, and returns a Config pointer.
// It supports both -server and -name for the server name (with -server taking precedence).
func ParseConfig() *Config {
	var cfg Config

	// Core uplink/server flags
	flag.StringVar(&cfg.Host, "host", "127.0.0.1", "uplink host")
	flag.IntVar(&cfg.Port, "port", 7000, "uplink port")

	// Accept BOTH -server and -name; -server wins if both provided
	var serverFlag, nameFlag string
	flag.StringVar(&serverFlag, "server", "", "our server name (alias for -name)")
	flag.StringVar(&nameFlag, "name", "services.emechnet.org", "our server name")

	flag.StringVar(&cfg.ServerDesc, "desc", "EmechNET IRC Services", "server description")
	flag.StringVar(&cfg.Password, "pass", "passwd", "link password")
	flag.StringVar(&cfg.SID, "sid", "702", "our 3-char server SID")

	flag.StringVar(&cfg.Proto, "proto", "insp4", "uplink protocol (insp4)")
	flag.StringVar(&cfg.LogLevel, "log", "info", "log level (debug|info|warn|error)")

	// Service client (Q)
	flag.StringVar(&cfg.QNick, "qnick", "Q", "service nick")
	flag.StringVar(&cfg.QHost, "qhost", "services.emechnet.org", "service host")
	flag.StringVar(&cfg.QUser, "quser", "qserv", "service ident")
	flag.StringVar(&cfg.QReal, "qreal", "Channel Service", "service realname")

	// Behavior
	flag.StringVar(&cfg.AutoJoin, "chan", "#feds", "autojoin channel")
	flag.StringVar(&cfg.AccountsFile, "accounts", "accounts.json", "accounts json file")

	flag.Parse()

	// Resolve -server vs -name
	cfg.ServerName = nameFlag
	if serverFlag != "" {
		cfg.ServerName = serverFlag
	}

	// Basic validation/sanity
	if len(cfg.SID) != 3 {
		panic(fmt.Errorf("SID must be exactly 3 characters, got %q", cfg.SID))
	}

	return &cfg
}
