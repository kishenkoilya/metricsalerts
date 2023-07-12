package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/caarlos0/env/v6"
	routing "github.com/go-ozzo/ozzo-routing/v2"
	"github.com/go-ozzo/ozzo-routing/v2/access"
	"github.com/go-ozzo/ozzo-routing/v2/fault"
	"github.com/go-ozzo/ozzo-routing/v2/slash"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
)

type Config struct {
	Address string `env:"ADDRESS"`
}

func printAllPage(storage *memstorage.MemStorage) routing.Handler {
	return func(c *routing.Context) error {
		path := strings.Trim(c.Request.URL.Path, "/")
		if path != "" {
			c.Response.WriteHeader(http.StatusNotFound)
			return c.Write([]byte(""))
		}
		c.Response.WriteHeader(http.StatusOK)
		return c.Write([]byte(storage.PrintAll()))
	}
}

func getPage(storage *memstorage.MemStorage) routing.Handler {
	return func(c *routing.Context) error {
		mType := c.Param("mType")
		mName := c.Param("mName")
		body := ""

		statusRes, err := validateValues(mType, mName)
		if err == nil {
			statusRes, body = getValue(storage, mType, mName)
		} else {
			body = err.Error()
		}
		c.Response.WriteHeader(statusRes)
		return c.Write([]byte(body))
	}
}

func updatePage(storage *memstorage.MemStorage) routing.Handler {
	return func(c *routing.Context) error {
		mType := c.Param("mType")
		mName := c.Param("mName")
		mVal := c.Param("mVal")
		body := "Update successful"

		statusRes, err := validateValues(mType, mName)
		if err == nil {
			statusRes = saveValues(storage, mType, mName, mVal)
		} else {
			body = err.Error()
		}
		c.Response.WriteHeader(statusRes)
		return c.Write([]byte(body))
	}
}

func validateValues(mType, mName string) (int, error) {
	if mType != "counter" && mType != "gauge" {
		return http.StatusBadRequest, errors.New("Metric type not counter, nor gauge!")
	}
	_, err := strconv.ParseInt(mName, 0, 64)
	if err == nil {
		return http.StatusBadRequest, err
	}
	_, err = strconv.ParseFloat(mName, 64)
	if err == nil {
		return http.StatusBadRequest, err
	}

	return http.StatusOK, nil
}

func saveValues(storage *memstorage.MemStorage, mType, mName, mVal string) int {
	if mType == "counter" {
		res, err := strconv.ParseInt(mVal, 0, 64)
		if err != nil {
			return http.StatusBadRequest
		}
		storage.PutCounter(mName, res)
	} else if mType == "gauge" {
		res, err := strconv.ParseFloat(mVal, 64)
		if err != nil {
			return http.StatusBadRequest
		}
		storage.PutGauge(mName, res)
	}
	return http.StatusOK
}

func getValue(storage *memstorage.MemStorage, mType, mName string) (int, string) {
	var res string
	status := http.StatusOK
	if mType == "gauge" {
		gauge, ok := storage.GetGauge(mName)
		if !ok {
			return http.StatusNotFound, ""
		}
		res = fmt.Sprint(gauge)
	} else {
		counter, ok := storage.GetCounter(mName)
		if !ok {
			return http.StatusNotFound, ""
		}
		res = fmt.Sprint(counter)
	}
	return status, res
}

func getVars() string {
	addr := flag.String("a", "localhost:8080", "An address the server will listen to")
	flag.Parse()
	fmt.Println(*addr)

	var cfg Config
	error := env.Parse(&cfg)
	if error != nil {
		log.Fatal(error)
	}
	if cfg.Address != "" {
		addr = &cfg.Address
	}
	fmt.Println(*addr)
	return *addr
}

func main() {
	addr := getVars()

	storage := memstorage.NewMemStorage()
	router := routing.New()

	router.Use(
		access.Logger(log.Printf),
		slash.Remover(http.StatusMovedPermanently),
		fault.Recovery(log.Printf),
	)

	router.Post("/update/<mType>/<mName>/<mVal>", updatePage(storage))
	router.Get("/value/<mType>/<mName>", getPage(storage))
	router.Get("/", printAllPage(storage))

	http.Handle("/", router)
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		panic(err)
	}
}
