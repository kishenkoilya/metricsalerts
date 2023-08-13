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
			// fmt.Println("Metric " + metricName + " equals " + fmt.Sprint(value) + " CanFloat: " + fmt.Sprint(value.CanFloat()) + " CanInt: " + fmt.Sprint(value.CanInt()) + " CanUint: " + fmt.Sprint(value.CanUint()))
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

func SendMetrics(addr *addressurl.AddressURL, storage *memstorage.MemStorage) {
	storage.Mutex.Lock()
	counters := storage.Counters
	gauges := storage.Gauges
	storage.Mutex.Unlock()
	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range counters {
		sendMetric(client, addr, metric, "counter", fmt.Sprint(value))
	}
	for metric, value := range gauges {
		sendMetric(client, addr, metric, "gauge", fmt.Sprint(value))
	}
}

func sendMetric(client *resty.Client, addr *addressurl.AddressURL, metric, mType, value string) {
	cli := client.R()
	resp, err := cli.Post(addr.AddrCommand("update", mType, metric, fmt.Sprint(value)))
	printResponse(resp, err, "sendMetric")
}

func SendAllMetrics(addr *addressurl.AddressURL, storage *memstorage.MemStorage) {
	storage.Mutex.Lock()
	counters := storage.Counters
	gauges := storage.Gauges
	storage.Mutex.Unlock()
	len := len(counters) + len(gauges)
	if len == 0 {
		return
	}

	metrics := make([]memstorage.Metrics, len)
	iter := 0
	for m, v := range counters {
		val := v
		metrics[iter] = memstorage.Metrics{
			ID:    m,
			MType: "counter",
			Value: nil,
			Delta: &val,
		}
		iter++
	}
	for m, v := range gauges {
		val := v
		metrics[iter] = memstorage.Metrics{
			ID:    m,
			MType: "gauge",
			Value: &val,
			Delta: nil,
		}
		iter++
	}

	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	request := makeJSONGZIPRequest(client, metrics)
	resp, err := request.Post(addr.AddrCommand("updates", "", "", ""))
	defer resp.RawBody().Close()
	printResponse(resp, err, "SendAllMetrics")
	// fmt.Println("result print")
	// var result []memstorage.Metrics
	// err = json.Unmarshal(resp.Body(), &result)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// for _, v := range result {
	// 	v.PrintMetric()
	// }
}

func SendJSONMetrics(addr *addressurl.AddressURL, storage *memstorage.MemStorage) {
	storage.Mutex.Lock()
	counters := storage.Counters
	gauges := storage.Gauges
	storage.Mutex.Unlock()

	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range counters {
		sendJSONMetric(client, addr, metric, &value, nil)
	}
	for metric, value := range gauges {
		sendJSONMetric(client, addr, metric, nil, &value)
	}
}

func sendJSONMetric(client *resty.Client, addr *addressurl.AddressURL, metric string, counter *int64, gauge *float64) {
	reqBody := memstorage.Metrics{
		ID: metric,
	}
	if counter == nil {
		reqBody.MType = "gauge"
		reqBody.Value = gauge
	} else {
		reqBody.MType = "counter"
		reqBody.Delta = counter
	}

	metrics := []memstorage.Metrics{reqBody}
	request := makeJSONGZIPRequest(client, metrics)

	resp, err := request.Post(addr.AddrCommand("update", "", "", ""))
	printResponse(resp, err, "sendJSONMetric")
}

func makeJSONGZIPRequest(client *resty.Client, reqBody []memstorage.Metrics) *resty.Request {
	var jsonData []byte
	var err error
	if len(reqBody) != 1 {
		jsonData, err = json.Marshal(reqBody)
	} else {
		jsonData, err = json.Marshal(reqBody[0])
	}
	fmt.Println(string(jsonData))
	if err != nil {
		fmt.Println(err)
		return nil
	}

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	_, err = gzipWriter.Write(jsonData)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	gzipWriter.Close()

	return client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Content-Encoding", "gzip").
		SetHeader("Accept-Encoding", "gzip").
		SetBody(&buf)
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
	printResponse(resp, err, "getJSONMetrics")
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
	printResponse(resp, err, "getAllMetrics")
	return resp
}

func printResponse(resp *resty.Response, err error, funcName string) {
	fmt.Println(resp.Request.URL)
	fmt.Println(resp.Proto() + " " + resp.Status())
	for k, v := range resp.Header() {
		fmt.Print(k + ": ")
		for _, s := range v {
			fmt.Print(fmt.Sprint(s))
		}
		fmt.Print("\n")
	}
	if err != nil {
		fmt.Println(funcName + " error: " + fmt.Sprint(err))
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
				testMass(&addr)
				// SendMetrics(&addr, storage)
				// SendJSONMetrics(&addr, storage)
				// SendAllMetrics(&addr, storage)
				// client := resty.New().R()
				// resp, err := client.Get(addr.AddrCommand("ping", "", "", ""))
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
				// time.Sleep(1 * time.Second)

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

func testMass(addr *addressurl.AddressURL) {
	delta1 := int64(837942796)
	value1 := float64(943708.7207719209)
	delta2 := int64(357569249)
	value2 := float64(31800.67860827374)
	delta3 := int64(837942796)
	value3 := float64(943708.7207719209)
	delta4 := int64(357569249)
	value4 := float64(31800.67860827374)
	metrics := []memstorage.Metrics{
		{ID: "CounterBatchZip23", MType: "counter", Delta: &delta1},
		{ID: "GaugeBatchZip142", MType: "gauge", Value: &value1},
		{ID: "CounterBatchZip23", MType: "counter", Delta: &delta2},
		{ID: "GaugeBatchZip142", MType: "gauge", Value: &value2},
		{ID: "CounterBatchZip23", MType: "counter", Delta: &delta3},
		{ID: "GaugeBatchZip142", MType: "gauge", Value: &value3},
		{ID: "CounterBatchZip23", MType: "counter", Delta: &delta4},
		{ID: "GaugeBatchZip142", MType: "gauge", Value: &value4},
	}
	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	request := makeJSONGZIPRequest(client, metrics)
	resp, err := request.Post(addr.AddrCommand("updates", "", "", ""))
	defer resp.RawBody().Close()
	printResponse(resp, err, "SendAllMetrics")
	fmt.Println("result print")
	var result []memstorage.Metrics
	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		fmt.Println(err)
	}
	for _, v := range result {
		v.PrintMetric()
	}

}
