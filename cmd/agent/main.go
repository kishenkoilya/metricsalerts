package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
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
	// SendJSONGauges(addr, storage)
	// SendJSONCounters(addr, storage)
	SendGauges(addr, storage)
	SendCounters(addr, storage)
}

func SendGauges(addr *addressurl.AddressURL, storage *memstorage.MemStorage) {
	storage.Mutex.Lock()
	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range storage.Gauges {
		cli := client.R()
		resp, err := cli.Post(addr.AddrCommand("update", "gauge", metric, fmt.Sprint(value)))
		fmt.Println(cli.RawRequest.URL)
		if err != nil {
			fmt.Println("SendGauges error: " + fmt.Sprint(err))
		} else {
			fmt.Println(resp.Proto() + " " + resp.Status())
			for k, v := range resp.Header() {
				fmt.Print(k + ": ")
				for _, s := range v {
					fmt.Print(fmt.Sprint(s))
				}
				fmt.Print("\n")
			}
			fmt.Println(string(resp.Body()))
		}
	}
	storage.Mutex.Unlock()
}

func SendCounters(addr *addressurl.AddressURL, storage *memstorage.MemStorage) {
	storage.Mutex.Lock()
	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range storage.Counters {
		cli := client.R()
		resp, err := cli.Post(addr.AddrCommand("update", "counter", metric, fmt.Sprint(value)))
		fmt.Println(cli.RawRequest.URL)
		if err != nil {
			fmt.Println("SendCounters error: " + fmt.Sprint(err))
		} else {
			fmt.Println(resp.Proto() + " " + resp.Status())
			for k, v := range resp.Header() {
				fmt.Print(k + ": ")
				for _, s := range v {
					fmt.Print(fmt.Sprint(s))
				}
				fmt.Print("\n")
			}
			fmt.Println(string(resp.Body()))
		}
	}
	storage.Mutex.Unlock()
}

func SendJSONGauges(addr *addressurl.AddressURL, storage *memstorage.MemStorage) {
	storage.Mutex.Lock()

	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range storage.Gauges {
		reqBody := memstorage.Metrics{
			ID:    metric,
			MType: "gauge",
			Value: &value,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			fmt.Println(err)
			return
		}

		var buf bytes.Buffer
		gzipWriter := gzip.NewWriter(&buf)
		_, err = gzipWriter.Write(jsonData)
		if err != nil {
			fmt.Println(err)
			return
		}
		gzipWriter.Close()

		request := client.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Content-Encoding", "gzip").
			SetHeader("Accept-Encoding", "gzip").
			SetBody(&buf)

		resp, err := request.Post(addr.AddrCommand("update", "", "", ""))
		if err != nil {
			fmt.Println("SendJSONGauges error: " + fmt.Sprint(err))
		} else {
			fmt.Println(resp.Proto() + " " + resp.Status())
			for k, v := range resp.Header() {
				fmt.Print(k + ": ")
				for _, s := range v {
					fmt.Print(fmt.Sprint(s))
				}
				fmt.Print("\n")
			}
		}
		fmt.Println(string(resp.Body()))
		// if strings.Contains(resp.Header().Get("Content-Encoding"), "gzip") {
		// 	responseData, err := decompressGzipResponse(resp.Body())
		// 	if err != nil {
		// 		fmt.Println("Error decompressing response:", err.Error())
		// 	}
		// 	fmt.Println(string(responseData))
		// }
	}

	storage.Mutex.Unlock()
}

func SendJSONCounters(addr *addressurl.AddressURL, storage *memstorage.MemStorage) {
	storage.Mutex.Lock()

	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range storage.Counters {
		reqBody := memstorage.Metrics{
			ID:    metric,
			MType: "counter",
			Delta: &value,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			fmt.Println(err)
			return
		}

		var buf bytes.Buffer
		gzipWriter := gzip.NewWriter(&buf)
		_, err = gzipWriter.Write(jsonData)
		if err != nil {
			fmt.Println(err)
			return
		}
		gzipWriter.Close()

		request := client.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Content-Encoding", "gzip").
			SetHeader("Accept-Encoding", "gzip").
			SetBody(&buf)

		resp, err := request.Post(addr.AddrCommand("update", "", "", ""))
		fmt.Println(resp.Proto() + " " + resp.Status())
		for k, v := range resp.Header() {
			fmt.Print(k + ": ")
			for _, s := range v {
				fmt.Print(fmt.Sprint(s))
			}
			fmt.Print("\n")
		}
		if err != nil {
			fmt.Println("SendJSONCounters error: " + fmt.Sprint(err))
		} else {
			fmt.Println(resp.Proto() + " " + resp.Status())
			for k, v := range resp.Header() {
				fmt.Print(k + ": ")
				for _, s := range v {
					fmt.Print(fmt.Sprint(s))
				}
				fmt.Print("\n")
			}
		}
		fmt.Println(string(resp.Body()))
	}
	storage.Mutex.Unlock()
}

func getJSONMetrics(mType, mName string, addr *addressurl.AddressURL, usegzip bool) *resty.Response {
	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	reqBody := memstorage.Metrics{ID: mName, MType: mType}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	request := client.R()
	if usegzip {
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
	if err != nil {
		fmt.Println("Error posting:", err.Error())
	}

	fmt.Println("request.Header:")
	for k, v := range request.Header {
		fmt.Print(k + ": ")
		for _, s := range v {
			fmt.Print(fmt.Sprint(s))
		}
		fmt.Print("\n")
	}
	fmt.Println(string(resp.Body()))
	return resp
}

func getMetric(mType, mName string, addr *addressurl.AddressURL) *resty.Response {
	client := resty.New()
	resp, err := client.R().Get(addr.AddrCommand("value", mType, mName, ""))
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func decompressGzipResponse(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func getAllMetrics(addr *addressurl.AddressURL) *resty.Response {
	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
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
				fmt.Println("Updating metrics")
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
				fmt.Println("Sending metrics")
				sendMetrics(storage, &addr)
				// resp := getAllMetrics(&addr)
				// fmt.Println(string(resp.Body()))
				// fmt.Println("all metr")
				// client := resty.New().R()
				// client.Header.Set("Accept", "html/text")
				// client.Header.Set("Accept-Encoding", "gzip")
				// resp, err := client.Get(addr.AddrEmpty())
				// if err != nil {
				// 	fmt.Println(err.Error())
				// }
				// fmt.Println(client.URL)
				// fmt.Println(string(resp.Body()))
				// // fmt.Println(client.RawRequest.URL)
				// fmt.Println(resp.Proto() + " " + resp.Status())
				// for k, v := range resp.Header() {
				// 	fmt.Print(k + ": ")
				// 	for _, s := range v {
				// 		fmt.Print(fmt.Sprint(s))
				// 	}
				// 	fmt.Print("\n")
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
