package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v6"
	routing "github.com/go-ozzo/ozzo-routing/v2"
	"github.com/go-ozzo/ozzo-routing/v2/fault"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
	"go.uber.org/zap"
)

var sugar zap.SugaredLogger

type Config struct {
	Address string `env:"ADDRESS"`
}

func LoggingMiddleware() routing.Handler {
	return func(c *routing.Context) error {
		start := time.Now()
		uri := c.Request.RequestURI
		method := c.Request.Method
		rw := &LogResponseWriter{c.Response, http.StatusOK, 0}
		c.Response = rw

		err := c.Next()

		duration := time.Since(start)
		sugar.Infoln(
			"uri", uri,
			"method", method,
			"status", rw.Status,
			"duration", duration,
			"size", rw.BytesWritten,
		)

		return err
	}
}

type LogResponseWriter struct {
	http.ResponseWriter
	Status       int
	BytesWritten int64
}

func (r *LogResponseWriter) Write(p []byte) (int, error) {
	written, err := r.ResponseWriter.Write(p)
	r.BytesWritten += int64(written)
	return written, err
}

func (r *LogResponseWriter) WriteHeader(status int) {
	r.Status = status
	r.ResponseWriter.WriteHeader(status)
}

func GzipHandle() routing.Handler {
	return func(c *routing.Context) error {
		if !strings.Contains(c.Request.Header.Get("Accept-Encoding"), "gzip") {
			return c.Next()
		}
		gzippedBuf := new(strings.Builder)
		gz := gzip.NewWriter(gzippedBuf)
		defer gz.Close()

		c.Response.Header().Set("Content-Encoding", "gzip")
		c.Response.Header().Set("Vary", "Accept-Encoding")

		c.Response = GzipWriter{c.Response, gz}

		if err := c.Next(); err != nil {
			return err
		}

		// Получаем данные из буфера и записываем их в ответ.
		gz.Flush()
		c.Response.Write([]byte(gzippedBuf.String()))
		return nil
	}
}

type GzipWriter struct {
	http.ResponseWriter
	Writer io.Writer
}

func (w GzipWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func printAllPage(storage *memstorage.MemStorage) routing.Handler {
	return func(c *routing.Context) error {
		path := strings.Trim(c.Request.URL.Path, "/")
		if path != "" {
			return c.WriteWithStatus([]byte(""), http.StatusNotFound)
		}
		return c.WriteWithStatus([]byte(storage.PrintAll()), http.StatusOK)
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
		return c.WriteWithStatus([]byte(body), statusRes)
	}
}

func getJSONPage(storage *memstorage.MemStorage) routing.Handler {
	return func(c *routing.Context) error {
		var statusRes int
		var req memstorage.Metrics

		reqBody := c.Request.Body
		if strings.Contains(c.Request.Header.Get("Content-Encoding"), "gzip") {
			var err error
			reqBody, err = gzip.NewReader(reqBody)
			if err != nil {
				sugar.Errorln("gzip.NewReader failed", err.Error())
				return c.WriteWithStatus([]byte(err.Error()), http.StatusInternalServerError)
			}
		}

		c.Response.Header().Set("Content-Type", "application/json")
		err := json.NewDecoder(reqBody).Decode(&req)
		if err != nil {
			return c.WriteWithStatus([]byte(err.Error()), http.StatusBadRequest)
		}

		_, err = validateValues(req.MType, req.ID)
		resp := &memstorage.Metrics{}
		if err == nil {
			statusRes, resp = storage.GetMetrics(req.MType, req.ID)
		} else {
			return c.WriteWithStatus([]byte(err.Error()), http.StatusBadRequest)
		}

		respJSON, err := json.Marshal(resp)
		if err != nil {
			sugar.Errorln("json.Marshal failed", err.Error())
			return c.WriteWithStatus([]byte(err.Error()), http.StatusInternalServerError)
		}
		return c.WriteWithStatus(respJSON, statusRes)
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
			statusRes = saveValue(storage, mType, mName, mVal)
		} else {
			body = err.Error()
		}
		return c.WriteWithStatus([]byte(body), statusRes)
	}
}

func updateJSONPage(storage *memstorage.MemStorage) routing.Handler {
	return func(c *routing.Context) error {
		var statusRes int
		var req *memstorage.Metrics
		c.Response.Header().Set("Content-Type", "application/json")

		reqBody := c.Request.Body
		if strings.Contains(c.Request.Header.Get("Content-Encoding"), "gzip") {
			var err error
			reqBody, err = gzip.NewReader(reqBody)
			if err != nil {
				return c.WriteWithStatus([]byte(err.Error()), http.StatusInternalServerError)
			}
		}

		err := json.NewDecoder(reqBody).Decode(&req)
		if err != nil {
			return c.WriteWithStatus([]byte(err.Error()), http.StatusBadRequest)
		}
		req.PrintMetrics()
		mType := req.MType
		mName := req.ID
		_, err = validateValues(mType, mName)
		if err == nil {
			statusRes, req = storage.SaveMetrics(req)
		} else {
			return c.WriteWithStatus([]byte(err.Error()), http.StatusBadRequest)
		}

		respJSON, err := json.Marshal(req)
		if err != nil {
			return c.WriteWithStatus([]byte(err.Error()), http.StatusInternalServerError)
		}
		return c.WriteWithStatus(respJSON, statusRes)
	}
}

func validateValues(mType, mName string) (int, error) {
	if mType != "counter" && mType != "gauge" {
		return http.StatusBadRequest, errors.New("metric type not counter, nor gauge")
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

func saveValue(storage *memstorage.MemStorage, mType, mName, mVal string) int {
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

	var cfg Config
	error := env.Parse(&cfg)
	if error != nil {
		log.Fatal(error)
	}
	if cfg.Address != "" {
		addr = &cfg.Address
	}
	return *addr
}

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		// вызываем панику, если ошибка
		panic(err)
	}
	defer logger.Sync()

	// делаем регистратор SugaredLogger
	sugar = *logger.Sugar()

	addr := getVars()

	storage := memstorage.NewMemStorage()
	router := routing.New()

	router.Use(
		LoggingMiddleware(),
		GzipHandle(),
		// access.Logger(log.Printf),
		// slash.Remover(http.StatusMovedPermanently),
		fault.Recovery(log.Printf),
	)

	router.Post("/update/", updateJSONPage(storage))
	router.Post("/value/", getJSONPage(storage))
	router.Post("/update/<mType>/<mName>/<mVal>", updatePage(storage))
	router.Get("/value/<mType>/<mName>", getPage(storage))
	router.Get("/", printAllPage(storage))

	http.Handle("/", router)

	sugar.Infow(
		"Starting server",
		"addr", addr,
	)

	err = http.ListenAndServe(addr, nil)
	if err != nil {
		sugar.Fatalw(err.Error(), "event", "start server")
	}
}
