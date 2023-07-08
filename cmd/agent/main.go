package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

type MemStorage struct {
	counters map[string]int64
	gauges   map[string]float64
}

type AddressURL struct {
	protocol string
	address  string
}

func (addr *AddressURL) AddrCommand(command, metricType, metricName, value string) string {
	if value == "" {
		return addr.protocol + "://" + addr.address + "/" + command + "/" + metricType + "/" + metricName
	}
	return addr.protocol + "://" + addr.address + "/" + command + "/" + metricType + "/" + metricName + "/" + value
}

func updateMetrics(m *runtime.MemStats, gaugeMetrics []string, storage *MemStorage) {
	runtime.ReadMemStats(m)
	for _, metricName := range gaugeMetrics {
		value := reflect.ValueOf(*m).FieldByName(metricName)
		if value.IsValid() {
			// fmt.Println("Metric " + metricName + " equals " + value.String())
			if value.CanFloat() {
				storage.gauges[metricName] = value.Float()
			} else if value.CanUint() {
				storage.gauges[metricName] = float64(value.Uint())
			}
		} else {
			fmt.Println("Metric named " + metricName + " was not found in MemStats")
		}
	}
	storage.counters["PollCount"]++
	storage.gauges["RandomValue"] = rand.Float64()
}

func sendMetrics(storage *MemStorage, addr *AddressURL) {
	client := resty.New()

	for metric, value := range storage.gauges {
		resp, err := client.R().Post(addr.AddrCommand("update", "gauge", metric, fmt.Sprint(value)))
		if err != nil {
			fmt.Println(err)
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
	}
	for metric, value := range storage.counters {
		resp, err := client.R().Post(addr.AddrCommand("update", "counter", metric, fmt.Sprint(value)))
		if err != nil {
			fmt.Println(err)
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
	}
}

func getMetrics(metricType, metricName string, addr *AddressURL) *resty.Response {
	client := resty.New()
	resp, err := client.R().Get(addr.AddrCommand("value", metricType, metricName, "")) //"http://localhost:8080/value/" + metricType + "/" + metricName)
	if err != nil {
		fmt.Println(err)
	}
	return resp
}

func main() {
	// ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	// defer cancel()
	ctx := context.Background()

	address := flag.String("a", "localhost:8080", "An address the server will listen to")
	reportInterval := flag.Int("r", 10, "An interval for sending metrics to server")
	pollInterval := flag.Int("p", 2, "An interval for collecting metrics")
	flag.Parse()

	addr := AddressURL{"http", *address}

	gaugeMetrics := []string{"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys", "HeapAlloc",
		"HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased", "HeapSys", "LastGC", "Lookups",
		"MCacheInuse", "MCacheSys", "MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
		"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys", "Sys", "TotalAlloc"}
	storage := MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)}
	var m runtime.MemStats

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(*pollInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// fmt.Println("Updating metrics")
				updateMetrics(&m, gaugeMetrics, &storage)
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(*reportInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// fmt.Println("Sending metrics")
				sendMetrics(&storage, &addr)
			case <-ctx.Done():
				return
			}
		}
	}()

	// cancel()

	wg.Wait()

	fmt.Println("Программа завершена")
}
