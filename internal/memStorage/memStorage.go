package MemStorage

import (
	"fmt"
	"sync"

	"github.com/go-resty/resty/v2"
	"github.com/kishenkoilya/metricsalerts/internal/AddressURL"
)

type MemStorage struct {
	Mutex    sync.RWMutex
	Counters map[string]int64
	Gauges   map[string]float64
}

func NewMemStorage() *MemStorage {
	return MemStorage{Mutex: sync.RWMutex{}, Counters: make(map[string]int64), Gauges: make(map[string]float64)}
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

func (m *MemStorage) SendGauges(addr *AddressURL.AddressURL) {
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

func (m *MemStorage) SendCounters(addr *AddressURL.AddressURL) {
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

func (m *MemStorage) PrintAll() string {
	m.Mutex.Lock()
	res := ""
	for k, v := range m.Counters {
		res += k + ": " + fmt.Sprint(v)
	}
	for k, v := range m.Gauges {
		res += k + ": " + fmt.Sprint(v)
	}
	m.Mutex.Unlock()
	return res
}