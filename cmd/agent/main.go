package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/go-resty/resty/v2"
	"github.com/kishenkoilya/metricsalerts/internal/addressurl"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
)

type Config struct {
	Address        string `env:"ADDRESS"`
	ReportInterval int    `env:"REPORT_INTERVAL"`
	PollInterval   int    `env:"POLL_INTERVAL"`
}

func updateMetrics(m *runtime.MemStats, metrics []string, storage *memstorage.MemStorage) error {
	runtime.ReadMemStats(m)
	for _, metricName := range metrics {
		value := reflect.ValueOf(*m).FieldByName(metricName)
		if value.IsValid() {
			// fmt.Println("Metric " + metricName + " equals " + value.String())
			if value.CanFloat() {
				storage.PutGauge(metricName, value.Float())
			} else if value.CanUint() {
				storage.PutGauge(metricName, float64(value.Uint()))
			}
		} else {
			err := errors.New("Metric named " + metricName + " was not found in MemStats")
			return err
		}
	}
	storage.PutCounter("PollCount", 1)
	storage.PutGauge("RandomValue", rand.Float64())
	return nil
}

func sendMetrics(storage *memstorage.MemStorage, addr *addressurl.AddressURL) {
	// storage.SendJSONGauges(addr)
	// storage.SendJSONCounters(addr)
	// storage.SendGauges(addr)
	storage.SendCounters(addr)
}

func getMetric(mType, mName string, addr *addressurl.AddressURL) *resty.Response {
	client := resty.New()
	resp, err := client.R().Get(addr.AddrCommand("value", mType, mName, ""))
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func getJSONMetrics(mType, mName string, addr *addressurl.AddressURL, usegzip bool) *resty.Response {
	client := resty.New()
	reqBody := memstorage.Metrics{ID: mName, MType: mType}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	request := client.R()
	if usegzip {
		fmt.Println("usegzip!!!!")
		var buf bytes.Buffer
		gzipWriter := gzip.NewWriter(&buf)
		_, err = gzipWriter.Write(jsonData)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		gzipWriter.Close()
		request.
			SetHeader("Content-Encoding", "gzip").
			SetHeader("Accept-Encoding", "gzip")
		jsonData = buf.Bytes()
	}

	request.
		SetHeader("Content-Type", "application/json").
		SetBody(jsonData)

	resp, err := request.Post(addr.AddrCommand("value", "", "", ""))

	fmt.Println("request.Header:")
	for k, v := range request.Header {
		fmt.Print(k + ": ")
		for _, s := range v {
			fmt.Print(fmt.Sprint(s))
		}
		fmt.Print("\n")
	}
	reqBody = memstorage.Metrics{ID: mName, MType: mType}
	json.Unmarshal(jsonData, &reqBody)
	reqBody.PrintMetrics()

	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func getAllMetrics(addr *addressurl.AddressURL) *resty.Response {
	client := resty.New()
	resp, err := client.R().Get(addr.AddrEmpty())
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func getVars() (string, int, int) {
	address := flag.String("a", "localhost:8080", "An address the server will listen to")
	reportInterval := flag.Int("r", 10, "An interval for sending metrics to server")
	pollInterval := flag.Int("p", 2, "An interval for collecting metrics")
	flag.Parse()

	var cfg Config
	error := env.Parse(&cfg)
	if error != nil {
		log.Fatal(error)
	}
	if cfg.Address != "" {
		address = &cfg.Address
	}
	if cfg.ReportInterval != 0 {
		reportInterval = &cfg.ReportInterval
	}
	if cfg.PollInterval != 0 {
		pollInterval = &cfg.PollInterval
	}
	return *address, *reportInterval, *pollInterval
}

func main() {
	// ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// defer cancel()
	ctx := context.Background()

	address, reportInterval, pollInterval := getVars()

	addr := addressurl.AddressURL{Protocol: "http", Address: address}

	metrics := []string{"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys", "HeapAlloc",
		"HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased", "HeapSys", "LastGC", "Lookups",
		"MCacheInuse", "MCacheSys", "MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
		"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys", "Sys", "TotalAlloc"}
	storage := memstorage.NewMemStorage()
	var m runtime.MemStats

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(pollInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// fmt.Println("Updating metrics")
				err := updateMetrics(&m, metrics, storage)
				if err != nil {
					panic(err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(reportInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// fmt.Println("Sending metrics")
				sendMetrics(storage, &addr)
				// resp := getAllMetrics(&addr)
				// fmt.Println(string(resp.Body()))

				// for k := range storage.Gauges {
				// 	response := getJSONMetrics("gauge", k, &addr)
				// 	fmt.Println(response.Proto() + " " + response.Status())
				// 	for k, v := range response.Header() {
				// 		fmt.Print(k + ": ")
				// 		for _, s := range v {
				// 			fmt.Print(fmt.Sprint(s))
				// 		}
				// 		fmt.Print("\n")
				// 	}
				// 	fmt.Println(string(response.Body()))
				// }
				// for k := range storage.Counters {
				// 	response := getJSONMetrics("counter", k, &addr, false)
				// 	fmt.Println("response proto: " + response.Proto() + " " + response.Status())
				// 	for k, v := range response.Header() {
				// 		fmt.Print(k + ": ")
				// 		for _, s := range v {
				// 			fmt.Print(fmt.Sprint(s))
				// 		}
				// 		fmt.Print("\n")
				// 	}
				// 	fmt.Println(string(response.Body()))
				// }
			case <-ctx.Done():
				return
			}
		}
	}()

	// cancel()

	wg.Wait()

	fmt.Println("Программа завершена")
}
