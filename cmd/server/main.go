package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	routing "github.com/go-ozzo/ozzo-routing/v2"
	"github.com/go-ozzo/ozzo-routing/v2/access"
	"github.com/go-ozzo/ozzo-routing/v2/fault"
	"github.com/go-ozzo/ozzo-routing/v2/slash"

	"github.com/caarlos0/env/v6"
)

func main() {
	var cfg Config
	error := env.Parse(&cfg)
	if error != nil {
		log.Fatal(error)
	}
	addr := cfg.Address

	if cfg.Address == "" {
		addr = *flag.String("a", "localhost:8080", "An address the server will listen to")
		flag.Parse()
	}
	fmt.Println(addr)

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
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		panic(err)
	}
}

type Config struct {
	Address string `env:"ADDRESS"`
}

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

func printAllPage(storage *memStorage) routing.Handler {
	return func(c *routing.Context) error {
		path := strings.Trim(c.Request.URL.Path, "/")
		if path != "" {
			c.Response.WriteHeader(http.StatusNotFound)
			return c.Write([]byte(""))
		}
		c.Response.WriteHeader(http.StatusOK)
		return c.Write([]byte(storage.printAll()))
	}
}

func getPage(storage *memStorage) routing.Handler {
	return func(c *routing.Context) error {
		mType := c.Param("mType")
		mName := c.Param("mName")
		body := ""

		statusRes := validateValues(mType, mName)
		if statusRes == http.StatusOK {
			statusRes, body = getValue(storage, mType, mName)
		} else {
			body = "Bad request"
		}
		c.Response.WriteHeader(statusRes)
		return c.Write([]byte(body))
	}
}

func updatePage(storage *memStorage) routing.Handler {
	return func(c *routing.Context) error {
		mType := c.Param("mType")
		mName := c.Param("mName")
		mVal := c.Param("mVal")
		body := "Update successful"

		statusRes := validateValues(mType, mName)
		if statusRes == http.StatusOK {
			statusRes = saveValues(storage, mType, mName, mVal)
		} else {
			body = "Bad request"
		}
		c.Response.WriteHeader(statusRes)
		return c.Write([]byte(body))
	}
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
