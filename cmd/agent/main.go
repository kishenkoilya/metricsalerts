package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/go-resty/resty/v2"
	"github.com/kishenkoilya/metricsalerts/internal/addressurl"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
)

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

	v, _ := mem.VirtualMemory()

	storage.PutGauge("TotalMemory", float64(v.Total))
	storage.PutGauge("FreeMemory", float64(v.Free))

	cpus, err := cpu.Percent(time.Second, true)
	if err != nil {
		panic(err)
	}
	for i, _ := cpu.Counts(true); i > 0; i-- {
		storage.PutGauge("CPUutilization"+fmt.Sprint(i), cpus[i-1])
	}

	storage.PutCounter("PollCount", 1)
	storage.PutGauge("RandomValue", rand.Float64())
	return nil
}

func SendMetrics(addr *addressurl.AddressURL, storage *memstorage.MemStorage, key string, rateLimit int, json bool) {
	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	ch := make(chan memstorage.Metrics, rateLimit)
	go fillMetricsChannel(ch, storage)

	for i := 0; i < rateLimit; i++ {
		if json {
			go metricJSONSender(i, client, addr, ch, key)
		} else {
			go metricSender(i, client, addr, ch)
		}
	}
}

func fillMetricsChannel(ch chan memstorage.Metrics, storage *memstorage.MemStorage) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		counters := storage.GetCounters()
		for name := range counters {
			stat, metric := storage.GetMetrics("counter", name)
			if stat != http.StatusOK {
				fmt.Println("Failed to get metric " + name)
				continue
			}
			ch <- *metric
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		gauges := storage.GetGauges()
		for name := range gauges {
			stat, metric := storage.GetMetrics("gauge", name)
			if stat != http.StatusOK {
				fmt.Println("Failed to get metric " + name)
				continue
			}
			ch <- *metric
		}
	}()
	wg.Wait()
	close(ch)
}

func metricSender(id int, client *resty.Client, addr *addressurl.AddressURL, ch chan memstorage.Metrics) {
	cli := client.R()
	for metric := range ch {
		var value string
		if metric.MType == "counter" {
			value = fmt.Sprint(*metric.Delta)
		} else {
			value = fmt.Sprint(*metric.Value)
		}
		resp, err := cli.Post(addr.AddrCommand("update", metric.MType, metric.ID, value))
		printResponse(resp, err, "metricSender id: "+fmt.Sprint(id))
	}
}

func metricJSONSender(id int, client *resty.Client, addr *addressurl.AddressURL, ch chan memstorage.Metrics, key string) {
	for metric := range ch {
		metrics := []memstorage.Metrics{metric}
		request := makeJSONGZIPRequest(client, metrics, key)

		resp, err := request.Post(addr.AddrCommand("update", "", "", ""))
		printResponse(resp, err, "metricJSONSender id: "+fmt.Sprint(id))
	}
}

func SendAllMetrics(addr *addressurl.AddressURL, storage *memstorage.MemStorage, key string) {
	counters := storage.GetCounters()
	gauges := storage.GetGauges()

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
	request := makeJSONGZIPRequest(client, metrics, key)
	fmt.Println(request.Header.Get("HashSHA256"))
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

func makeJSONGZIPRequest(client *resty.Client, reqBody []memstorage.Metrics, key string) *resty.Request {
	var jsonData []byte
	var err error
	if len(reqBody) != 1 {
		jsonData, err = json.Marshal(reqBody)
	} else {
		jsonData, err = json.Marshal(reqBody[0])
	}
	// fmt.Println(string(jsonData))
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
	request := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Content-Encoding", "gzip").
		SetHeader("Accept-Encoding", "gzip").
		SetBody(&buf)
	if key != "" {
		sign := generateHMACSHA256(jsonData, key)
		request.SetHeader("HashSHA256", string(sign))
	}
	return request
}

func generateHMACSHA256(data []byte, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func getJSONMetrics(mType, mName string, addr *addressurl.AddressURL, usegzip bool, key string) *resty.Response {
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

	if key != "" {
		sign := generateHMACSHA256(jsonData, key)
		request.SetHeader("HashSHA256", string(sign))
	}

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
	var respString string
	respString += "printResponse: " + funcName + "\n"
	respString += resp.Request.URL + "\n"
	if err != nil {
		respString += funcName + " error: " + fmt.Sprint(err) + "\n"
	} else {
		respString += resp.Proto() + " " + resp.Status() + "\n"
		for k, v := range resp.Header() {
			respString += k + ": "
			for _, s := range v {
				respString += fmt.Sprint(s)
			}
			respString += "\n"
		}
	}
	respString += string(resp.Body()) + "\n"
	respString += "printResponse END: " + funcName + "\n"
	fmt.Println(respString)
	// if strings.Contains(resp.Header().Get("Content-Encoding"), "gzip") {
	// 	responseData, err := decompressGzipResponse(resp.Body())
	// 	if err != nil {
	// 		fmt.Println("Error decompressing response:", err.Error())
	// 	}
	// 	fmt.Println(string(responseData))
	// }
}

func main() {
	// ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// defer cancel()
	ctx := context.Background()

	config := getVars()

	addr := addressurl.AddressURL{Protocol: "http", Address: (*config).Address}

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
		ticker := time.NewTicker(time.Duration((*config).PollInterval) * time.Second)
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
		ticker := time.NewTicker(time.Duration((*config).ReportInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				fmt.Println("Sending metrics")
				// testMass(&addr)
				SendMetrics(&addr, storage, config.Key, config.RateLimit, false)
				// SendMetrics(&addr, storage, config.Key, config.RateLimit, true)
				// SendAllMetrics(&addr, storage, (*config).Key)
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
	request := makeJSONGZIPRequest(client, metrics, "")
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
