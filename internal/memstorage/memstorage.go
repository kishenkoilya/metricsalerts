package memstorage

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/go-resty/resty/v2"
	"github.com/kishenkoilya/metricsalerts/internal/addressurl"
)

type MemStorage struct {
	Mutex    sync.RWMutex
	Counters map[string]int64
	Gauges   map[string]float64
}

type Metrics struct {
	ID    string   `json:"id"`              // имя метрики
	MType string   `json:"type"`            // параметр, принимающий значение gauge или counter
	Delta *int64   `json:"delta,omitempty"` // значение метрики в случае передачи counter
	Value *float64 `json:"value,omitempty"` // значение метрики в случае передачи gauge
}

func decompressGzipResponse(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func (m *Metrics) PrintMetrics() {
	if m.Delta != nil {
		fmt.Println("ID: " + m.ID + "; MType: " + m.MType + "; Delta: " + fmt.Sprint(*m.Delta) + "; Value:" + fmt.Sprint(m.Value))
	} else if m.Value != nil {
		fmt.Println("ID: " + m.ID + "; MType: " + m.MType + "; Delta: " + fmt.Sprint(m.Delta) + "; Value:" + fmt.Sprint(*m.Value))
	} else {
		fmt.Println("ID: " + m.ID + "; MType: " + m.MType + "; Delta: " + fmt.Sprint(m.Delta) + "; Value:" + fmt.Sprint(m.Value))
	}
}

func (m *MemStorage) SaveMetrics(metric *Metrics) (int, *Metrics) {
	if metric.MType == "gauge" {
		m.PutGauge(metric.ID, *metric.Value)
		val, ok := m.GetGauge(metric.ID)
		if ok {
			*metric.Value = val
		}
	} else if metric.MType == "counter" {
		m.PutCounter(metric.ID, *metric.Delta)
		val, ok := m.GetCounter(metric.ID)
		if ok {
			*metric.Delta = val
		}
	} else {
		return http.StatusBadRequest, metric
	}
	return http.StatusOK, metric
}

func (m *MemStorage) GetMetrics(mType, mName string) (int, *Metrics) {
	var res Metrics
	res.ID = mName
	res.MType = mType
	if mType == "gauge" {
		val, ok := m.GetGauge(mName)
		if ok {
			res.Value = &val
		} else {
			return http.StatusNotFound, &res
		}
	} else if mType == "counter" {
		del, ok := m.GetCounter(mName)
		if ok {
			res.Delta = &del
		} else {
			return http.StatusNotFound, &res
		}
	}
	return http.StatusOK, &res
}

func NewMemStorage() *MemStorage {
	return &MemStorage{Mutex: sync.RWMutex{}, Counters: make(map[string]int64), Gauges: make(map[string]float64)}
}

func (m *MemStorage) PutCounter(nameC string, value int64) {
	m.Mutex.Lock()
	m.Counters[nameC] += value
	m.Mutex.Unlock()
}

func (m *MemStorage) PutGauge(nameG string, value float64) {
	m.Mutex.Lock()
	m.Gauges[nameG] = value
	m.Mutex.Unlock()
}

func (m *MemStorage) GetCounter(nameC string) (int64, bool) {
	m.Mutex.Lock()
	res, ok := m.Counters[nameC]
	m.Mutex.Unlock()
	return res, ok
}

func (m *MemStorage) GetGauge(nameG string) (float64, bool) {
	m.Mutex.Lock()
	res, ok := m.Gauges[nameG]
	m.Mutex.Unlock()
	return res, ok
}

func (m *MemStorage) SendGauges(addr *addressurl.AddressURL) {
	m.Mutex.Lock()

	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range m.Gauges {
		resp, err := client.R().Post(addr.AddrCommand("update", "gauge", metric, fmt.Sprint(value)))
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

	m.Mutex.Unlock()
}

func (m *MemStorage) SendCounters(addr *addressurl.AddressURL) {
	m.Mutex.Lock()

	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range m.Counters {
		resp, err := client.R().Post(addr.AddrCommand("update", "counter", metric, fmt.Sprint(value)))
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

	m.Mutex.Unlock()
}

func (m *MemStorage) SendJSONGauges(addr *addressurl.AddressURL) {
	m.Mutex.Lock()

	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range m.Gauges {
		reqBody := Metrics{
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
			// fmt.Println(resp.Proto() + " " + resp.Status())
			// for k, v := range resp.Header() {
			// 	fmt.Print(k + ": ")
			// 	for _, s := range v {
			// 		fmt.Print(fmt.Sprint(s))
			// 	}
			// 	fmt.Print("\n")
			// }
			// fmt.Println(string(resp.Body()))
			fmt.Println(string(resp.Body()))
			responseData, err := decompressGzipResponse(resp.Body())
			if err != nil {
				fmt.Println("Error decompressing response:", err.Error())
			}
			fmt.Println(string(responseData))

		}
	}

	m.Mutex.Unlock()
}

func (m *MemStorage) SendJSONCounters(addr *addressurl.AddressURL) {
	m.Mutex.Lock()

	client := resty.NewWithClient(&http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	})
	for metric, value := range m.Counters {
		reqBody := Metrics{
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
		if err != nil {
			fmt.Println("SendJSONCounters error: " + fmt.Sprint(err))
		} else {
			// fmt.Println(resp.Proto() + " " + resp.Status())
			// for k, v := range resp.Header() {
			// 	fmt.Print(k + ": ")
			// 	for _, s := range v {
			// 		fmt.Print(fmt.Sprint(s))
			// 	}
			// 	fmt.Print("\n")
			// }
			// fmt.Println(string(resp.Body()))
			fmt.Println(string(resp.Body()))
			responseData, err := decompressGzipResponse(resp.Body())
			if err != nil {
				fmt.Println("Error decompressing response:", err.Error())
			}
			fmt.Println(string(responseData))
		}
	}

	m.Mutex.Unlock()
}

func (m *MemStorage) PrintAll() string {
	m.Mutex.Lock()
	fmt.Println("Getting all vals")
	res := ""
	if len(m.Counters) > 0 {
		res += "Counters:\n"
	}
	for k, v := range m.Counters {
		res += k + ": " + fmt.Sprint(v) + "\n"
	}
	if len(m.Counters) > 0 {
		res += "Gauges:\n"
	}
	for k, v := range m.Gauges {
		res += k + ": " + fmt.Sprint(v) + "\n"
	}
	m.Mutex.Unlock()
	return res
}
