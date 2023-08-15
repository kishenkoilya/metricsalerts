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
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx"
	"github.com/julienschmidt/httprouter"
	"github.com/kishenkoilya/metricsalerts/internal/filerw"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
	"github.com/kishenkoilya/metricsalerts/internal/psqlinteraction"
	"go.uber.org/zap"
)

var sugar zap.SugaredLogger

type Config struct {
	Address       string `env:"ADDRESS"`
	StoreInterval int    `env:"STORE_INTERVAL"`
	FilePath      string `env:"FILE_STORAGE_PATH"`
	Restore       bool   `env:"RESTORE"`
	DatabaseDSN   string `env:"DATABASE_DSN"`
}

type HandlerVars struct {
	storage         *memstorage.MemStorage
	syncFileWriter  *filerw.Producer
	psqlConnectLine *string
	db              *psqlinteraction.DBConnection
}

func ParamsMiddleware(next httprouter.Handle, handlerVars *HandlerVars) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx := context.WithValue(r.Context(), HandlerVars{}, handlerVars)
		next(w, r.WithContext(ctx), ps)
	})
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

func Retrypg(errClass string, f psqlinteraction.RetryFunc) (interface{}, error) {
	var result interface{}
	var err error

	for i := 0; i < 3; i++ {
		result, err = f()
		if err == nil {
			return result, nil
		} else {
			if pgerr, ok := err.(pgx.PgError); ok {
				if errCodeCompare(errClass, pgerr.Code) {
					switch i {
					case 0:
						time.Sleep(1 * time.Second)
					case 1:
						time.Sleep(3 * time.Second)
					case 2:
						time.Sleep(5 * time.Second)
					default:
						return nil, err
					}
				}
			}
			return nil, err
		}
	}

	return nil, fmt.Errorf("all %d attempts failed: %w", 3, err)
}

func errCodeCompare(errClass, errCode string) bool {
	switch errClass {
	case pgerrcode.ConnectionException:
		return pgerrcode.IsConnectionException(errCode)
	case pgerrcode.OperatorIntervention:
		return pgerrcode.IsOperatorIntervention(errCode)
	default:
		return false
	}
}

func printAllPage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	handlerVars := r.Context().Value(HandlerVars{}).(*HandlerVars)
	sugar.Infoln("printAllPage")
	path := strings.Trim(r.URL.Path, "/")
	if path != "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(handlerVars.storage.PrintAll()))
	w.WriteHeader(http.StatusOK)
}

func getPage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	handlerVars := r.Context().Value(HandlerVars{}).(*HandlerVars)
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
	statusRes, body = getValue(handlerVars.storage, mType, mName)
	if statusRes != http.StatusOK {
		// sugar.Errorln("getValue error: ", err.Error())
		http.Error(w, "Error getting value", statusRes)
		return
	}

	w.Write([]byte(body))
	w.WriteHeader(statusRes)
}

func pingPostgrePage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	handlerVars := r.Context().Value(HandlerVars{}).(*HandlerVars)
	sugar.Infoln("pingPostgrePage")
	dbPingFunc := psqlinteraction.PingPSQL(*handlerVars.psqlConnectLine)
	_, err := Retrypg(pgerrcode.OperatorIntervention, dbPingFunc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func updatePage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	handlerVars := r.Context().Value(HandlerVars{}).(*HandlerVars)
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
	metric := memstorage.NewMetric(mType, mName, mVal)
	if metric == nil {
		http.Error(w, "Error parsing value", http.StatusBadRequest)
		return
	}
	statusRes, metric = handlerVars.storage.SaveMetric(metric)
	if statusRes != http.StatusOK {
		// sugar.Errorln("saveValue error: ", err.Error())
		http.Error(w, "Error parsing value", statusRes)
		return
	}
	statusRes = writeValue(handlerVars, mType, mName, mVal)
	if statusRes != http.StatusOK {
		// sugar.Errorln("saveValue error: ", err.Error())
		http.Error(w, "Error writing value to storage", statusRes)
		return
	}
	body += metric.StringMetric()
	w.Write([]byte(body))
	w.WriteHeader(statusRes)
}

func getJSONPage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	handlerVars := r.Context().Value(HandlerVars{}).(*HandlerVars)
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
	req.PrintMetric()

	_, err = validateValues(req.MType, req.ID)
	resp := &memstorage.Metrics{}
	if err != nil {
		http.Error(w, "json.Marshal failed", http.StatusBadRequest)
		return
	}

	statusRes, resp = handlerVars.storage.GetMetrics(req.MType, req.ID)
	if statusRes != http.StatusOK {
		// sugar.Errorln("storage.GetMetrics failed: ", statusRes)
		w.WriteHeader(statusRes)
		return
	}
	resp.PrintMetric()

	respJSON, err := json.Marshal(resp)
	if err != nil {
		// sugar.Errorln("json.Marshal failed: ", err.Error())
		http.Error(w, "json.Marshal failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(statusRes)
	w.Write(respJSON)
}

func updateJSONPage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	handlerVars := r.Context().Value(HandlerVars{}).(*HandlerVars)
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
	statusRes, req = handlerVars.storage.SaveMetric(req)
	if statusRes != http.StatusOK {
		http.Error(w, "storage.SaveMetrics failed", statusRes)
		return
	}
	var mVal string
	if req.Delta != nil {
		mVal = fmt.Sprint(req.Delta)
	} else {
		mVal = fmt.Sprint(req.Value)
	}
	statusRes = writeValue(handlerVars, mType, mName, mVal)
	if statusRes != http.StatusOK {
		// sugar.Errorln("saveValue error: ", err.Error())
		http.Error(w, "Error writing value to storage", statusRes)
		return
	}
	respJSON, err := json.Marshal(&req)
	if err != nil {
		http.Error(w, "gzip.NewReader failed", http.StatusInternalServerError)
		return
	}

	sugar.Infoln(string(respJSON))

	w.WriteHeader(statusRes)
	w.Write(respJSON)
}

func massUpdatePage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	handlerVars := r.Context().Value(HandlerVars{}).(*HandlerVars)
	sugar.Infoln("massUpdatePage")
	var statusRes int
	var req *[]memstorage.Metrics
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
	// for _, val := range *req {
	// 	val.PrintMetric()
	// }

	statusRes, resp := handlerVars.storage.SaveMetrics(req)
	if statusRes != http.StatusOK {
		http.Error(w, "storage.SaveMetrics failed", statusRes)
		return
	}
	fmt.Println("printing response: ")
	for _, val := range *resp {
		val.PrintMetric()
	}
	statusRes = writeValues(handlerVars, resp)
	if statusRes != http.StatusOK {
		http.Error(w, "writeValues failed", statusRes)
		return
	}
	resp1 := (*resp)[0]
	respJSON, err := json.Marshal(&resp1)
	if err != nil {
		http.Error(w, "json.Marshal failed", http.StatusInternalServerError)
		return
	}

	sugar.Infoln(string(respJSON))

	w.WriteHeader(statusRes)
	w.Write(respJSON)
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

func writeValue(handlerVars *HandlerVars, mType, mName, mVal string) int {
	if handlerVars.db != nil {
		sugar.Infoln("Writing metric to db")
		dbWriteMetricFunc := handlerVars.db.WriteMetric(mType, mName, mVal)
		_, err := Retrypg(pgerrcode.ConnectionException, dbWriteMetricFunc)
		if err != nil {
			fmt.Println(err.Error())
			return http.StatusInternalServerError
		}
	} else if handlerVars.syncFileWriter != nil {
		sugar.Infoln("Writing metric to file")
		err := handlerVars.syncFileWriter.WriteMetric(&filerw.Metric{ID: mName, MType: mType, MVal: mVal})
		if err != nil {
			fmt.Println(err.Error())
			return http.StatusInternalServerError
		}
	}
	return http.StatusOK
}

func writeValues(handlerVars *HandlerVars, metrics *[]memstorage.Metrics) int {
	if handlerVars.db != nil {
		sugar.Infoln("Writing metrics to db")
		dbWriteMetricsFunc := handlerVars.db.WriteMetrics(metrics)
		_, err := Retrypg(pgerrcode.ConnectionException, dbWriteMetricsFunc)
		if err != nil {
			fmt.Println(err.Error())
			return http.StatusInternalServerError
		}
	} else if handlerVars.syncFileWriter != nil {
		sugar.Infoln("Writing metric to file")
		err := handlerVars.syncFileWriter.WriteMetrics(metrics)
		if err != nil {
			fmt.Println(err.Error())
			return http.StatusInternalServerError
		}
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

func getVars() (string, int, string, bool, string) {
	addr := flag.String("a", "localhost:8080", "An address the server will listen to")
	storeInterval := flag.Int("i", 300, "A time interval for storing metrics in file")
	filePath := flag.String("f", "/tmp/metrics-db.json", "Path to file where metrics will be stored")
	restore := flag.Bool("r", true, "A flag that determines wether server will download metrics from file upon start")
	psqlLine := flag.String("d", "", "A string that contains info to connect to psql")

	flag.Parse()

	var cfg Config

	error := env.Parse(&cfg)
	if error != nil {
		log.Fatal(error)
	}
	if cfg.Address != "" {
		addr = &cfg.Address
	}
	if _, err := os.LookupEnv("STORE_INTERVAL"); err {
		storeInterval = &cfg.StoreInterval
	}
	if cfg.FilePath != "" {
		filePath = &cfg.FilePath
	}
	if _, err := os.LookupEnv("RESTORE"); err {
		restore = &cfg.Restore
	}
	if cfg.DatabaseDSN != "" {
		psqlLine = &cfg.DatabaseDSN
	}
	return *addr, *storeInterval, *filePath, *restore, *psqlLine
}

func main() {
	ctx := context.Background()
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	sugar = *logger.Sugar()

	addr, storeInterval, filePath, restore, psqlLine := getVars()
	fmt.Println(addr, storeInterval, filePath, restore, psqlLine)
	storage := memstorage.NewMemStorage()
	dbConnFunc := psqlinteraction.NewDBConnection(psqlLine)
	if restore {
		obj, err := Retrypg(pgerrcode.OperatorIntervention, dbConnFunc)
		if err != nil {
			fmt.Println(err.Error())
			consumer, err := filerw.NewConsumer(filePath)
			if err == nil {
				storage, _ = consumer.ReadMemStorage()
				fmt.Println(storage.PrintAll())
			}
		} else {
			var db *psqlinteraction.DBConnection
			if obj != nil {
				db = obj.(*psqlinteraction.DBConnection)
			}
			dbReadMemFunc := db.ReadMemStorage()
			obj, err := Retrypg(pgerrcode.ConnectionException, dbReadMemFunc)
			if obj != nil {
				storage = obj.(*memstorage.MemStorage)
			}
			if err == nil {
				fmt.Println(storage.PrintAll())
			} else {
				consumer, err := filerw.NewConsumer(filePath)
				if err == nil {
					storage, _ = consumer.ReadMemStorage()
					fmt.Println(storage.PrintAll())
				}
			}
		}
	}

	// psqlLine = "host=localhost port=5432 user=postgres password=gpadmin dbname=postgres"
	var handlerVars *HandlerVars
	syncFileWriter, err := filerw.NewProducer(filePath, false)
	if err != nil {
		sugar.Fatalw(err.Error(), "event", "Init file writer")
	}
	if storeInterval != 0 {
		syncFileWriter = nil
	}
	obj, err := Retrypg(pgerrcode.OperatorIntervention, dbConnFunc)
	var db *psqlinteraction.DBConnection
	if obj != nil {
		db = obj.(*psqlinteraction.DBConnection)
	}
	if err != nil || storeInterval != 0 {
		handlerVars = &HandlerVars{
			storage:         storage,
			syncFileWriter:  syncFileWriter,
			psqlConnectLine: &psqlLine,
			db:              nil,
		}
	} else {
		handlerVars = &HandlerVars{
			storage:         storage,
			syncFileWriter:  syncFileWriter,
			psqlConnectLine: &psqlLine,
			db:              db,
		}
	}
	if db != nil {
		dbInitFunc := db.InitTables()
		_, err := Retrypg(pgerrcode.ConnectionException, dbInitFunc)
		if err != nil {
			sugar.Fatalw(err.Error(), "event", "init DB")
		}
	}

	router := httprouter.New()
	router.GET("/", LoggingMiddleware(GzipMiddleware(ParamsMiddleware(printAllPage, handlerVars))))
	router.GET("/value/:mType/:mName", LoggingMiddleware(GzipMiddleware(ParamsMiddleware(getPage, handlerVars))))
	router.GET("/ping", LoggingMiddleware(GzipMiddleware(ParamsMiddleware(pingPostgrePage, handlerVars))))
	router.POST("/update/:mType/:mName/:mVal", LoggingMiddleware(GzipMiddleware(ParamsMiddleware(updatePage, handlerVars))))
	router.POST("/value/", LoggingMiddleware(GzipMiddleware(ParamsMiddleware(getJSONPage, handlerVars))))
	router.POST("/update/", LoggingMiddleware(GzipMiddleware(ParamsMiddleware(updateJSONPage, handlerVars))))
	router.POST("/updates/", LoggingMiddleware(GzipMiddleware(ParamsMiddleware(massUpdatePage, handlerVars))))

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

	if storeInterval != 0 {
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
					obj, err := Retrypg(pgerrcode.OperatorIntervention, dbConnFunc)
					var db *psqlinteraction.DBConnection
					if obj != nil {
						db = obj.(*psqlinteraction.DBConnection)
					}
					if err != nil {
						producer, err := filerw.NewProducer(filePath, true)
						if err != nil {
							sugar.Fatalw(err.Error(), "event", "init file writer")
						}
						err = producer.WriteMemStorage(storage)
						if err != nil {
							panic(err)
						}
					} else {
						dbWriteMemFunc := db.WriteMemStorage(storage)
						_, err = Retrypg(pgerrcode.ConnectionException, dbWriteMemFunc)
						if err != nil {
							panic(err)
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		wg.Wait()
	}
	waitForShutdown(server, handlerVars, filePath)
	fmt.Println("Программа завершена")
}

func waitForShutdown(server *http.Server, handlerVars *HandlerVars, filePath string) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	<-signalChan
	dbConnFunc := psqlinteraction.NewDBConnection(*handlerVars.psqlConnectLine)
	obj, err := Retrypg(pgerrcode.OperatorIntervention, dbConnFunc)
	var db *psqlinteraction.DBConnection
	if obj != nil {
		db = obj.(*psqlinteraction.DBConnection)
	}
	if err != nil {
		producer, err := filerw.NewProducer(filePath, true)
		if err != nil {
			sugar.Fatalw(err.Error(), "event", "init file writer")
		}
		err = server.Shutdown(context.TODO())
		if err != nil {
			sugar.Errorf("Ошибка при остановке HTTP-сервера: %v\n", err)
		}
		err = producer.WriteMemStorage(handlerVars.storage)
		if err != nil {
			sugar.Errorln(err.Error())
		}
	} else {
		dbWriteMemFunc := db.WriteMemStorage(handlerVars.storage)
		_, err = Retrypg(pgerrcode.ConnectionException, dbWriteMemFunc)
		if err != nil {
			panic(err)
		}
	}
	fmt.Println("HTTP-сервер остановлен.")
}
