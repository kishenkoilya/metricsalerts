package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	// "time"
	// "runtime"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc(`/update/`, updatePage)
	mux.HandleFunc(`/`, mainPage)

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

	// header := fmt.Sprintf("")
	// if statusRes != http.StatusOK {
	// 	fmt.Println("NOT OK")
	// } else {
	// 	dateFormatted := time.Now().Format("Mon, 02 Jan 2006 15:04:05 MST")
	// 	header += req.Proto + " " + fmt.Sprint(statusRes) + " OK\n"
	// 	header += "Date: " + fmt.Sprint(dateFormatted) + "\n"
	// 	header += "Content-Length: " + fmt.Sprint(0) + "\n"
	// 	header += "Content-Type: text/plain; charset=utf-8"
	// }

	// header := fmt.Sprintf("path: \n")
	// for k, v := range storage.counters {
	// 	header += k + ": " + fmt.Sprint(v) + "\n"
	// }
	// for k, v := range storage.gauges {
	// 	header += k + ": " + fmt.Sprint(v) + "\n"
	// }
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
			return http.StatusNotFound
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
			return http.StatusNotFound
		}
		storage.gauges[pathSplit[3]] = res
	}
	return http.StatusOK
}

func mainPage(res http.ResponseWriter, req *http.Request) {
	header := fmt.Sprintf("Method: %s\r\n", req.Method)
	header += "Header ===============\r\n"
	for k, v := range req.Header {
		header += fmt.Sprintf("%s: %v\r\n", k, v)
	}
	header += "Query parameters ===============\r\n"
	if err := req.ParseForm(); err != nil {
		res.Write([]byte(err.Error()))
		return
	}
	for k, v := range req.Form {
		header += fmt.Sprintf("%s: %v\r\n", k, v)
	}
	header += "OTHERS ===============\r\n"
	header += req.Host + "\n"
	header += req.Proto + "\n"
	header += req.RemoteAddr + "\n"
	header += req.RequestURI + "\n"
	header += req.URL.Path + "\n"
	res.Write([]byte(header))
}
