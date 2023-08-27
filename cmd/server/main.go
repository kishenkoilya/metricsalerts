package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx"
	"github.com/julienschmidt/httprouter"
	"github.com/kishenkoilya/metricsalerts/internal/filerw"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
	"github.com/kishenkoilya/metricsalerts/internal/psqlinteraction"
	"go.uber.org/zap"
)

var sugar zap.SugaredLogger

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
