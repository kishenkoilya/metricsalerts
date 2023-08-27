package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/caarlos0/env/v6"
)

type Config struct {
	Address        string `env:"ADDRESS"`
	ReportInterval int    `env:"REPORT_INTERVAL"`
	PollInterval   int    `env:"POLL_INTERVAL"`
	Key            string `env:"KEY"`
}

func getVars() Config {
	address := flag.String("a", "localhost:8080", "An address the server will listen to")
	reportInterval := flag.Int("r", 10, "An interval for sending metrics to server")
	pollInterval := flag.Int("p", 2, "An interval for collecting metrics")
	key := flag.String("k", "", "Key for hash func")

	flag.Parse()

	var cfg Config
	error := env.Parse(&cfg)
	if error != nil {
		log.Fatal(error)
	}
	if cfg.Address == "" {
		cfg.Address = *address
	}
	if cfg.ReportInterval == 0 {
		cfg.ReportInterval = *reportInterval
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = *pollInterval
	}
	if cfg.Key == "" {
		cfg.Key = *key
	}
	return cfg
}

func (conf *Config) printConfig() {
	fmt.Printf("Address: %s; Report Interval: %d; Poll Interval: %d; Key: %s",
		conf.Address, conf.ReportInterval, conf.PollInterval, conf.Key)
}
