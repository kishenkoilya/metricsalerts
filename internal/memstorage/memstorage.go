package memstorage

import (
	"fmt"
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

func (m *MemStorage) GetMetrics(mType, mName string) (int, *Metrics) {
	var res Metrics
	res.ID = mType
	res.MType = mName
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

	client := resty.New()
	for metric, value := range m.Gauges {
		resp, err := client.R().Post(addr.AddrCommand("update", "gauge", metric, fmt.Sprint(value)))
		if err != nil {
			panic(err)
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

	m.Mutex.Unlock()
}

func (m *MemStorage) SendCounters(addr *addressurl.AddressURL) {
	m.Mutex.Lock()

	client := resty.New()
	for metric, value := range m.Counters {
		resp, err := client.R().Post(addr.AddrCommand("update", "gauge", metric, fmt.Sprint(value)))
		if err != nil {
			panic(err)
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

	m.Mutex.Unlock()
}

func (m *MemStorage) SendJsonGauges(addr *addressurl.AddressURL) {
	m.Mutex.Lock()

	client := resty.New()
	for metric, value := range m.Gauges {
		reqBody := Metrics{
			ID:    metric,
			MType: "gauge",
			Value: &value,
		}
		request := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(reqBody)

		resp, err := request.Post(addr.AddrCommand("update", "", "", ""))
		if err != nil {
			panic(err)
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

	m.Mutex.Unlock()
}

func (m *MemStorage) SendJsonCounters(addr *addressurl.AddressURL) {
	m.Mutex.Lock()

	client := resty.New()
	for metric, value := range m.Counters {
		reqBody := Metrics{
			ID:    metric,
			MType: "gauge",
			Delta: &value,
		}
		request := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(reqBody)

		resp, err := request.Post(addr.AddrCommand("update", "", "", ""))
		if err != nil {
			panic(err)
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

	m.Mutex.Unlock()
}

func (m *MemStorage) PrintAll() string {
	m.Mutex.Lock()
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
