package main

import (
	"flag"
	"log"
	"os"

	"github.com/caarlos0/env/v6"
)

type Config struct {
	Address       string `env:"ADDRESS"`
	StoreInterval int    `env:"STORE_INTERVAL"`
	FilePath      string `env:"FILE_STORAGE_PATH"`
	Restore       bool   `env:"RESTORE"`
	DatabaseDSN   string `env:"DATABASE_DSN"`
	Key           string `env:"KEY"`
}

func getVars() (string, int, string, bool, string, string) {
	addr := flag.String("a", "localhost:8080", "An address the server will listen to")
	storeInterval := flag.Int("i", 300, "A time interval for storing metrics in file")
	filePath := flag.String("f", "/tmp/metrics-db.json", "Path to file where metrics will be stored")
	restore := flag.Bool("r", true, "A flag that determines wether server will download metrics from file upon start")
	psqlLine := flag.String("d", "", "A string that contains info to connect to psql")
	key := flag.String("k", "", "Key for hash func")

	flag.Parse()

	var cfg Config

	error := env.Parse(&cfg)
	if error != nil {
		log.Fatal(error)
	}
	if cfg.Address != "" {
		addr = &cfg.Address
	}
	if _, err := os.LookupEnv("STORE_INTERVAL"); err {
		storeInterval = &cfg.StoreInterval
	}
	if cfg.FilePath != "" {
		filePath = &cfg.FilePath
	}
	if _, err := os.LookupEnv("RESTORE"); err {
		restore = &cfg.Restore
	}
	if cfg.DatabaseDSN != "" {
		psqlLine = &cfg.DatabaseDSN
	}
	if cfg.Key != "" {
		key = &cfg.Key
	}

	return *addr, *storeInterval, *filePath, *restore, *psqlLine, *key
}
