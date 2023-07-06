package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	routing "github.com/go-ozzo/ozzo-routing/v2"
	"github.com/go-ozzo/ozzo-routing/v2/access"
	"github.com/go-ozzo/ozzo-routing/v2/fault"
	"github.com/go-ozzo/ozzo-routing/v2/slash"
)

type memStorage struct {
	counters map[string]int64
	gauges   map[string]float64
}

func (m *memStorage) putCounter(nameC string, value int64) {
	m.counters[nameC] += value
}

func (m *memStorage) putGauge(nameG string, value float64) {
	m.gauges[nameG] = value
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
	router := routing.New()

	router.Use(
		access.Logger(log.Printf),
		slash.Remover(http.StatusMovedPermanently),
		fault.Recovery(log.Printf),
	)

	router.Post("/update/<mType>/<mName>/<mVal>", updatePage(&storage))
	router.Get("/value/<mType>/<mName>", getPage(&storage))
	router.Get("/", printAllPage(&storage))

	http.Handle("/", router)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}

func printAllPage(storage *memStorage) routing.Handler {
	return func(c *routing.Context) error {
		log.Println("printAll" + c.Request.Method)
		if c.Request.Method != http.MethodGet {
			c.Response.WriteHeader(http.StatusNotFound)

			return c.Write([]byte(""))
		}
		pathSplit := strings.Split(c.Request.URL.Path, "/")
		for _, v := range pathSplit {
			if v != "" {
				c.Response.WriteHeader(http.StatusNotFound)

				return c.Write([]byte(""))
			}
		}
		c.Response.WriteHeader(http.StatusOK)
		return c.Write([]byte(storage.printAll()))
	}
}

func getPage(storage *memStorage) routing.Handler {
	return func(c *routing.Context) error {
		log.Println("printAll" + c.Request.Method)
		body := ""
		statusRes, mType, mName, _ := parsePath(c.Request.URL.Path)
		if statusRes == http.StatusOK {
			statusRes = validateValues(mType, mName)
			if statusRes == http.StatusOK && c.Request.Method == http.MethodGet {
				statusRes, body = getValue(storage, mType, mName)
			} else {
				statusRes = http.StatusBadRequest
				c.Response.Write([]byte("NOT GET"))
			}
		}
		c.Response.WriteHeader(statusRes)
		return c.Write([]byte(body))
	}
}

func updatePage(storage *memStorage) routing.Handler {
	return func(c *routing.Context) error {
		log.Println("UPDATE" + c.Request.Method)
		body := ""
		statusRes, mType, mName, mVal := parsePath(c.Request.URL.Path)
		log.Println(fmt.Sprint(statusRes) + " " + mType + " " + mName + " " + mVal)
		if statusRes == http.StatusOK {
			statusRes = validateValues(mType, mName)
			log.Println(fmt.Sprint(statusRes) + " " + mType + " " + mName + " " + mVal)
			if statusRes == http.StatusOK && c.Request.Method == http.MethodPost {
				statusRes = saveValues(storage, mType, mName, mVal)
				log.Println(fmt.Sprint(statusRes) + " " + mType + " " + mName + " " + mVal)
			} else {
				statusRes = http.StatusBadRequest
				body = "NOT POST NOR GET"
			}
		}
		c.Response.WriteHeader(statusRes)
		return c.Write([]byte(body))
	}
}

func parsePath(path string) (int, string, string, string) {
	pathSplit := strings.Split(path, "/")
	if len(pathSplit) < 4 || pathSplit[3] == "" {
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
			return http.StatusBadRequest
		}
		storage.putCounter(mName, res)
	} else if mType == "gauge" {
		res, err := strconv.ParseFloat(mVal, 64)
		if err != nil {
			return http.StatusBadRequest
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
