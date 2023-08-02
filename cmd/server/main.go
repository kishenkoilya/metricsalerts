package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/julienschmidt/httprouter"
	"github.com/kishenkoilya/metricsalerts/internal/filerw"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
	"go.uber.org/zap"
)

var storage *memstorage.MemStorage
var sugar zap.SugaredLogger
var syncFileWriter *filerw.Producer

type Config struct {
	Address       string `env:"ADDRESS"`
	StoreInterval int    `env:"STORE_INTERVAL"`
	FilePath      string `env:"FILE_STORAGE_PATH"`
	Restore       bool   `env:"RESTORE"`
}

func LoggingMiddleware(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		start := time.Now()
		uri := r.RequestURI
		method := r.Method
		rw := &LogResponseWriter{ResponseWriter: w, StatusCode: http.StatusOK}
		w = rw

		next(w, r, ps)

		duration := time.Since(start)
		sugar.Infoln(
			"uri", uri,
			"method", method,
			"status", rw.StatusCode,
			"duration", duration,
			"size", rw.Size,
			"Accept-Encoding", r.Header.Get("Accept-Encoding"),
		)
	})
}

type LogResponseWriter struct {
	http.ResponseWriter
	StatusCode int
	Size       int
	IsWritten  bool
}

func (lrw *LogResponseWriter) Write(b []byte) (int, error) {
	if !lrw.IsWritten {
		lrw.IsWritten = true
		lrw.StatusCode = http.StatusOK
	}
	size, err := lrw.ResponseWriter.Write(b)
	lrw.Size += size // Увеличиваем размер ответа на количество записанных байт
	return size, err
}

// Переопределение WriteHeader метода для записи статуса ответа
func (lrw *LogResponseWriter) WriteHeader(statusCode int) {
	if !lrw.IsWritten {
		lrw.StatusCode = statusCode
		lrw.ResponseWriter.WriteHeader(statusCode)
		lrw.IsWritten = true
	}
}

func GzipMiddleware(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next(w, r, ps)
			return
		}
		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err != nil {
			io.WriteString(w, err.Error())
			return
		}
		defer gz.Close()

		w.Header().Set("Content-Encoding", "gzip")

		next(gzipWriter{ResponseWriter: w, Writer: gz}, r, ps)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	Writer io.Writer
}

func (w gzipWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func printAllPage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	sugar.Infoln("printAllPage")
	path := strings.Trim(r.URL.Path, "/")
	if path != "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(storage.PrintAll()))
	w.WriteHeader(http.StatusOK)
}

func getPage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	sugar.Infoln("getPage")
	mType := ps.ByName("mType")
	mName := ps.ByName("mName")
	body := ""

	statusRes, err := validateValues(mType, mName)
	if err != nil {
		sugar.Errorln("validateValues error: ", err.Error())
		// sugar.Errorln("validateValues error: ", err.Error())
		http.Error(w, "Error validating type and name", statusRes)
		return
	}
	statusRes, body = getValue(storage, mType, mName)
	if statusRes != http.StatusOK {
		// sugar.Errorln("getValue error: ", err.Error())
		http.Error(w, "Error getting value", statusRes)
		return
	}
	// if strings.Contains(c.Request.Header.Get("Accept-Encoding"), "gzip") {
	// 	c.Response.Header().Set("Content-Encoding", "gzip")
	// 	gz := gzip.NewWriter(c.Response)
	// 	defer gz.Close()
	// 	_, err := gz.Write([]byte(body))
	// 	if err != nil {
	// 		sugar.Errorln("gzip write failed: ", err.Error())
	// 	}
	// }
	w.Write([]byte(body))
	w.WriteHeader(statusRes)
}

func updatePage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	sugar.Infoln("updatePage")
	mType := ps.ByName("mType")
	mName := ps.ByName("mName")
	mVal := ps.ByName("mVal")
	body := "Update successful"

	statusRes, err := validateValues(mType, mName)
	if err != nil {
		// sugar.Errorln("validateValues error: ", err.Error())
		http.Error(w, "Error validating type and name", statusRes)
		return
	}
	statusRes = saveValue(storage, mType, mName, mVal)
	if statusRes != http.StatusOK {
		// sugar.Errorln("saveValue error: ", err.Error())
		http.Error(w, "Error parsing value", statusRes)
		return
	}

	// if strings.Contains(c.Request.Header.Get("Accept-Encoding"), "gzip") {
	// 	c.Response.Header().Set("Content-Encoding", "gzip")
	// 	gz := gzip.NewWriter(c.Response)
	// 	defer gz.Close()
	// 	_, err := gz.Write([]byte(body))
	// 	if err != nil {
	// 		sugar.Errorln("gzip write failed: ", err.Error())
	// 	}
	// }
	w.Write([]byte(body))
	w.WriteHeader(statusRes)
}

func getJSONPage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	sugar.Infoln("getJSONPage")
	var statusRes int
	var req memstorage.Metrics

	reqBody := r.Body
	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
		var err error
		reqBody, err = gzip.NewReader(reqBody)
		if err != nil {
			// sugar.Errorln("gzip.NewReader failed", err.Error())
			http.Error(w, "gzip.NewReader failed", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewDecoder(reqBody).Decode(&req)
	if err != nil {
		http.Error(w, "json.Marshal failed", http.StatusBadRequest)
		return
	}

	// for k, v := range c.Request.Header {
	// 	fmt.Print(k + ": ")
	// 	for _, s := range v {
	// 		fmt.Print(fmt.Sprint(s))
	// 	}
	// 	fmt.Print("\n")
	// }
	// req.PrintMetrics()

	_, err = validateValues(req.MType, req.ID)
	resp := &memstorage.Metrics{}
	if err != nil {
		http.Error(w, "json.Marshal failed", http.StatusBadRequest)
		return
	}

	statusRes, resp = storage.GetMetrics(req.MType, req.ID)
	if statusRes != http.StatusOK {
		// sugar.Errorln("storage.GetMetrics failed: ", statusRes)
		w.WriteHeader(statusRes)
		return
	}

	respJSON, err := json.Marshal(resp)
	if err != nil {
		// sugar.Errorln("json.Marshal failed: ", err.Error())
		http.Error(w, "json.Marshal failed", http.StatusInternalServerError)
		return
	}
	// if strings.Contains(c.Request.Header.Get("Accept-Encoding"), "gzip") {
	// 	c.Response.Header().Set("Content-Encoding", "gzip")
	// 	gz := gzip.NewWriter(c.Response)
	// 	defer gz.Close()
	// 	_, err := gz.Write(respJSON)
	// 	if err != nil {
	// 		sugar.Errorln("gzip write failed: ", err.Error())
	// 	}
	// }
	w.Write(respJSON)
	// w.WriteHeader(statusRes)
}

func updateJSONPage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	sugar.Infoln("updateJSONPage")
	var statusRes int
	var req *memstorage.Metrics
	w.Header().Set("Content-Type", "application/json")

	reqBody := r.Body
	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
		var err error
		reqBody, err = gzip.NewReader(reqBody)
		if err != nil {
			http.Error(w, "gzip.NewReader failed", http.StatusInternalServerError)
			return
		}
	}

	err := json.NewDecoder(reqBody).Decode(&req)
	if err != nil {
		http.Error(w, "json.Marshal failed", http.StatusBadRequest)
		return
	}
	// req.PrintMetrics()
	mType := req.MType
	mName := req.ID
	_, err = validateValues(mType, mName)
	if err != nil {
		http.Error(w, "json.Marshal failed", http.StatusBadRequest)
		return
	}
	statusRes, req = storage.SaveMetrics(req)
	if statusRes != http.StatusOK {
		http.Error(w, "storage.SaveMetrics failed", statusRes)
		return
	}
	respJSON, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "gzip.NewReader failed", http.StatusInternalServerError)
		return
	}
	// if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
	// 	w.Header().Set("Content-Encoding", "gzip")
	// 	gz := gzip.NewWriter(w)
	// 	defer gz.Close()
	// 	gz.Write(respJSON)
	// 	return
	// }
	sugar.Infoln(string(respJSON))

	w.Write(respJSON)
	// w.WriteHeader(statusRes)
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
	if syncFileWriter != nil {
		syncFileWriter.WriteMetric(&filerw.Metric{ID: mName, MType: mType, MVal: mVal})
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

func getVars() (string, int, string, bool) {
	addr := flag.String("a", "localhost:8080", "An address the server will listen to")
	storeInterval := flag.Int("i", 300, "A time interval for storing metrics in file")
	filePath := flag.String("f", "/tmp/metrics-db.json", "Path to file where metrics will be stored")
	restore := flag.Bool("r", true, "A flag that determines wether server will download metrics from file upon start")

	flag.Parse()

	var cfg Config

	error := env.Parse(&cfg)
	if error != nil {
		log.Fatal(error)
	}
	if cfg.Address != "" {
		addr = &cfg.Address
	}
	if _, err := os.LookupEnv("STORE_INTERVAL"); err == true {
		storeInterval = &cfg.StoreInterval
	}
	if cfg.FilePath != "" {
		filePath = &cfg.FilePath
	}
	if _, err := os.LookupEnv("RESTORE"); err == true {
		restore = &cfg.Restore
	}
	return *addr, *storeInterval, *filePath, *restore
}

func main() {
	ctx := context.Background()
	logger, err := zap.NewDevelopment()
	if err != nil {
		// вызываем панику, если ошибка
		panic(err)
	}
	defer logger.Sync()

	// делаем регистратор SugaredLogger
	sugar = *logger.Sugar()

	addr, storeInterval, filePath, restore := getVars()
	fmt.Println(addr, storeInterval, filePath, restore)
	storage = memstorage.NewMemStorage()
	if restore {
		_, err := os.Open(filePath)
		if err == nil {
			consumer, err := filerw.NewConsumer(filePath)
			if err == nil {
				storage, _ = consumer.ReadMemStorage()
				fmt.Println(storage.PrintAll())
			}
		}
	}

	router := httprouter.New()
	router.GET("/", LoggingMiddleware(GzipMiddleware(printAllPage)))
	router.GET("/value/:mType/:mName", LoggingMiddleware(GzipMiddleware(getPage)))
	router.POST("/update/:mType/:mName/:mVal", LoggingMiddleware(GzipMiddleware(updatePage)))
	router.POST("/value/", LoggingMiddleware(GzipMiddleware(getJSONPage)))
	router.POST("/update/", LoggingMiddleware(GzipMiddleware(updateJSONPage)))

	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}
	go func() {
		err = server.ListenAndServe()
		if err != nil {
			sugar.Fatalw(err.Error(), "event", "start server")
		}
	}()
	if storeInterval == 0 {
		syncFileWriter, err = filerw.NewProducer(filePath, false)
		if err != nil {
			sugar.Fatalw(err.Error(), "event", "init file writer")
		}
	} else {
		var wg sync.WaitGroup
		wg.Add(10)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Duration(storeInterval) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					fmt.Println("Saving to storage")
					producer, err := filerw.NewProducer(filePath, true)
					if err != nil {
						sugar.Fatalw(err.Error(), "event", "init file writer")
					}
					err = producer.WriteMemStorage(storage)
					if err != nil {
						panic(err)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		wg.Wait()
	}
	waitForShutdown(server, filePath)
	fmt.Println("Программа завершена")
}

func waitForShutdown(server *http.Server, filePath string) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	<-signalChan
	producer, err := filerw.NewProducer(filePath, true)
	if err != nil {
		sugar.Fatalw(err.Error(), "event", "init file writer")
	}
	err = server.Shutdown(nil)
	if err != nil {
		sugar.Errorf("Ошибка при остановке HTTP-сервера: %v\n", err)
	}
	err = producer.WriteMemStorage(storage)
	if err != nil {
		sugar.Errorln(err.Error())
	}
	fmt.Println("HTTP-сервер остановлен.")
}
