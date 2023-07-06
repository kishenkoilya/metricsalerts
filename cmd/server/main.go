package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type memStorage struct {
	counters map[string]int64
	gauges   map[string]float64
}

func (m *memStorage) putCounter(nameC string, value int64) {
	m.counters[nameC] += value
}

func (m *memStorage) putGauge(nameG string, value float64) {
	m.gauges[nameG] += value
}

func (m *memStorage) getCounter(nameC string) (int64, bool) {
	res, ok := m.counters[nameC]
	return res, ok
}

func (m *memStorage) getGauge(nameG string) (float64, bool) {
	res, ok := m.gauges[nameG]
	return res, ok
}

func (m *memStorage) printAll() string {
	res := ""
	for k, v := range m.counters {
		res += k + ": " + fmt.Sprint(v)
	}
	for k, v := range m.gauges {
		res += k + ": " + fmt.Sprint(v)
	}
	return res
}

func main() {
	storage := memStorage{counters: make(map[string]int64), gauges: make(map[string]float64)}
	mux := http.NewServeMux()
	mux.HandleFunc(`/update/`, func(res http.ResponseWriter, req *http.Request) {
		updatePage(res, req, &storage)
	})
	mux.HandleFunc(`/value/`, func(res http.ResponseWriter, req *http.Request) {
		getPage(res, req, &storage)
	})
	mux.HandleFunc(`/`, func(res http.ResponseWriter, req *http.Request) {
		printAllPage(res, req, &storage)
	})

	err := http.ListenAndServe(`:8080`, mux)
	if err != nil {
		panic(err)
	}
}

func printAllPage(res http.ResponseWriter, req *http.Request, storage *memStorage) {
	res.WriteHeader(http.StatusOK)
	res.Write([]byte(storage.printAll()))
}

func getPage(res http.ResponseWriter, req *http.Request, storage *memStorage) {
	body := ""
	statusRes, mType, mName, _ := parsePath(req.URL.Path)
	if statusRes == http.StatusOK {
		statusRes = validateValues(mType, mName)
		if statusRes == http.StatusOK && req.Method == http.MethodGet {
			statusRes, body = getValue(storage, mType, mName)
		} else {
			statusRes = http.StatusBadRequest
			res.Write([]byte("NOT GET"))
		}
	}
	res.WriteHeader(statusRes)
	res.Write([]byte(body))
}

func updatePage(res http.ResponseWriter, req *http.Request, storage *memStorage) {
	body := ""
	statusRes, mType, mName, mVal := parsePath(req.URL.Path)
	fmt.Println(fmt.Sprint(statusRes) + " " + mType + " " + mName + " " + mVal)
	if statusRes == http.StatusOK {
		statusRes = validateValues(mType, mName)
		fmt.Println(fmt.Sprint(statusRes) + " " + mType + " " + mName + " " + mVal)
		if statusRes == http.StatusOK && req.Method == http.MethodPost {
			statusRes = saveValues(storage, mType, mName, mVal)
			fmt.Println(fmt.Sprint(statusRes) + " " + mType + " " + mName + " " + mVal)
		} else {
			statusRes = http.StatusBadRequest
			body = "NOT POST NOR GET"
		}
	}
	res.WriteHeader(statusRes)
	res.Write([]byte(body))
}

func parsePath(path string) (int, string, string, string) {
	pathSplit := strings.Split(path, "/")
	if len(pathSplit) < 4 {
		return http.StatusNotFound, "", "", ""
	} else if len(pathSplit) < 5 {
		return http.StatusOK, pathSplit[2], pathSplit[3], ""
	}
	return http.StatusOK, pathSplit[2], pathSplit[3], pathSplit[4]
}

func validateValues(mType, mName string) int {
	if mType != "counter" && mType != "gauge" {
		return http.StatusBadRequest
	}
	_, err := strconv.ParseInt(mName, 0, 64)
	if err == nil {
		return http.StatusBadRequest
	}
	_, err = strconv.ParseFloat(mName, 64)
	if err == nil {
		return http.StatusBadRequest
	}

	return http.StatusOK
}

func saveValues(storage *memStorage, mType, mName, mVal string) int {
	if mType == "counter" {
		res, err := strconv.ParseInt(mVal, 0, 64)
		if err != nil {
			return http.StatusNotFound
		}
		storage.putCounter(mName, res)
	} else if mType == "gauge" {
		res, err := strconv.ParseFloat(mVal, 64)
		if err != nil {
			return http.StatusNotFound
		}
		storage.putGauge(mName, res)
	}
	return http.StatusOK
}

func getValue(storage *memStorage, mType, mName string) (int, string) {
	var res string
	status := http.StatusOK
	if mType == "gauge" {
		gauge, ok := storage.getGauge(mName)
		if !ok {
			return http.StatusNotFound, ""
		}
		res = fmt.Sprint(gauge)
	} else {
		counter, ok := storage.getCounter(mName)
		if !ok {
			return http.StatusNotFound, ""
		}
		res = fmt.Sprint(counter)
	}
	return status, res
}
