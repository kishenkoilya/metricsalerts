package main

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/kishenkoilya/metricsalerts/internal/filerw"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
	"github.com/kishenkoilya/metricsalerts/internal/psqlinteraction"
)

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
