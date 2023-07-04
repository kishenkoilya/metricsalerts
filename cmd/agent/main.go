package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"runtime"
	"time"
)

type MemStorage struct {
	counters map[string]int64
	gauges   map[string]float64
}

func updateMetrics(m *runtime.MemStats, gaugeMetrics []string, storage *MemStorage) {
	runtime.ReadMemStats(m)
	for _, metricName := range gaugeMetrics {
		value := reflect.ValueOf(*m).FieldByName(metricName)
		if value.IsValid() {
			fmt.Println("Metric " + metricName + " equals " + value.String())
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
	rand.Seed(time.Now().UnixNano())
	storage.gauges["RandomValue"] = rand.Float64()
}

func sendMetrics(storage *MemStorage) {
	for metric, value := range storage.gauges {
		resp, err := http.Post("http://localhost:8080/update/gauge/"+metric+"/"+fmt.Sprint(value), "text/plain", http.NoBody)
		if err != nil {
			panic(err)
		} else {
			fmt.Println(resp.Proto + " " + resp.Status)
			for k, v := range resp.Header {
				fmt.Print(k + ": ")
				for _, s := range v {
					fmt.Print(fmt.Sprint(s))
				}
				fmt.Print("\n")
			}
		}
		resp.Body.Close()
	}
}

func main() {
	gaugeMetrics := []string{"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys", "HeapAlloc",
		"HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased", "HeapSys", "LastGC", "Lookups",
		"MCacheInuse", "MCacheSys", "MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
		"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys", "Sys", "TotalAlloc"}
	storage := MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)}
	var m runtime.MemStats

	var i int = 0
	for {
		updateMetrics(&m, gaugeMetrics, &storage)
		i++
		time.Sleep(2 * time.Second)
		if i%5 == 0 {
			sendMetrics(&storage)
			i = 0
		}
	}

}
