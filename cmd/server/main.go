package main

import (
	"net/http"
	"strconv"
	"strings"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc(`/update/`, updatePage)

	err := http.ListenAndServe(`:8080`, mux)
	if err != nil {
		panic(err)
	}
}

type MemStorage struct {
	counters map[string]int64
	gauges   map[string]float64
}

func updatePage(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		res.Write([]byte("NOT POST"))
		return
	}
	storage := MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)}
	pathSplit := strings.Split(req.URL.Path, "/")
	statusRes := parsePath(pathSplit, &storage)

	body := "dfgdgfsdds"

	res.WriteHeader(statusRes)
	res.Write([]byte(body))
}

func parsePath(pathSplit []string, storage *MemStorage) int {
	if pathSplit[2] == "counter" {
		if len(pathSplit) < 5 {
			return http.StatusNotFound
		}
		_, err := strconv.ParseInt(pathSplit[3], 0, 64)
		if err == nil {
			return http.StatusNotFound
		}
		res, err := strconv.ParseInt(pathSplit[4], 0, 64)
		if err != nil {
			return http.StatusBadRequest
		}
		storage.counters[pathSplit[3]] += res
	} else if pathSplit[2] == "gauge" {
		if len(pathSplit) < 5 {
			return http.StatusNotFound
		}
		_, err := strconv.ParseFloat(pathSplit[3], 64)
		if err == nil {
			return http.StatusNotFound
		}
		res, err := strconv.ParseFloat(pathSplit[4], 64)
		if err != nil {
			return http.StatusBadRequest
		}
		storage.gauges[pathSplit[3]] = res
	} else {
		return http.StatusBadRequest
	}
	return http.StatusOK
}
