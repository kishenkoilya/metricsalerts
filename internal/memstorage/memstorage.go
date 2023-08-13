package memstorage

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
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

func NewMetric(mType, mName, mVal string) *Metrics {
	res := Metrics{
		MType: mType,
		ID:    mName,
	}
	if mType == "gauge" {
		val, err := strconv.ParseFloat(mVal, 64)
		if err != nil {
			fmt.Println(err.Error())
			return nil
		}
		res.Value = &val
	} else if mType == "counter" {
		val, err := strconv.ParseInt(mVal, 0, 64)
		if err != nil {
			fmt.Println(err.Error())
			return nil
		}
		res.Delta = &val
	}
	return &res
}

func (m *Metrics) PrintMetric() {
	if m.Delta != nil {
		fmt.Println("ID: " + m.ID + "; MType: " + m.MType + "; Delta: " + fmt.Sprint(*m.Delta) + "; Value:" + fmt.Sprint(m.Value))
	} else if m.Value != nil {
		fmt.Println("ID: " + m.ID + "; MType: " + m.MType + "; Delta: " + fmt.Sprint(m.Delta) + "; Value:" + fmt.Sprint(*m.Value))
	} else {
		fmt.Println("ID: " + m.ID + "; MType: " + m.MType + "; Delta: " + fmt.Sprint(m.Delta) + "; Value:" + fmt.Sprint(m.Value))
	}
}

func (m *Metrics) StringMetric() string {
	if m.Delta != nil {
		return "ID: " + m.ID + "; MType: " + m.MType + "; Delta: " + fmt.Sprint(*m.Delta) + "; Value:" + fmt.Sprint(m.Value)
	} else if m.Value != nil {
		return "ID: " + m.ID + "; MType: " + m.MType + "; Delta: " + fmt.Sprint(m.Delta) + "; Value:" + fmt.Sprint(*m.Value)
	} else {
		return "ID: " + m.ID + "; MType: " + m.MType + "; Delta: " + fmt.Sprint(m.Delta) + "; Value:" + fmt.Sprint(m.Value)
	}
}

func (m *MemStorage) SaveMetric(metric *Metrics) (int, *Metrics) {
	metric.PrintMetric()
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

func (m *MemStorage) SaveMetrics(metrics *[]Metrics) (int, *[]Metrics) {
	status := http.StatusOK
	for i, metric := range *metrics {
		var pMetric *Metrics
		status, pMetric = m.SaveMetric(&metric)
		if status != http.StatusOK {
			return status, nil
		}
		(*metrics)[i] = *pMetric
	}
	return status, metrics
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
