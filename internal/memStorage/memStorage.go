package memstorage

import (
	"fmt"
	"sync"

	"github.com/go-resty/resty/v2"
)

type memStorage struct {
	mutex    sync.RWMutex
	counters map[string]int64
	gauges   map[string]float64
}

func (m *memStorage) putCounter(nameC string, value int64) {
	m.mutex.Lock()
	m.counters[nameC] += value
	m.mutex.Unlock()
}

func (m *memStorage) putGauge(nameG string, value float64) {
	m.mutex.Lock()
	m.gauges[nameG] = value
	m.mutex.Unlock()
}

func (m *memStorage) getCounter(nameC string) (int64, bool) {
	m.mutex.Lock()
	res, ok := m.counters[nameC]
	m.mutex.Unlock()
	return res, ok
}

func (m *memStorage) getGauge(nameG string) (float64, bool) {
	m.mutex.Lock()
	res, ok := m.gauges[nameG]
	m.mutex.Unlock()
	return res, ok
}

func (m *memStorage) sendGauges(addr *AddressURL) {
	m.mutex.Lock()

	client := resty.New()
	for metric, value := range m.gauges {
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

	m.mutex.Unlock()
}

func (m *memStorage) sendCounters(addr *AddressURL) {
	m.mutex.Lock()

	client := resty.New()
	for metric, value := range m.counters {
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

	m.mutex.Unlock()
}

func (m *memStorage) printAll() string {
	m.mutex.Lock()
	res := ""
	for k, v := range m.counters {
		res += k + ": " + fmt.Sprint(v)
	}
	for k, v := range m.gauges {
		res += k + ": " + fmt.Sprint(v)
	}
	m.mutex.Unlock()
	return res
}
