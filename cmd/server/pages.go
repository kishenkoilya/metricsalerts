package main

import (
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/julienschmidt/httprouter"
	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
	"github.com/kishenkoilya/metricsalerts/internal/psqlinteraction"
)

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

	bodyBytes, err := io.ReadAll(reqBody)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	if headerSign := r.Header.Get("HashSHA256"); headerSign != "" {
		sign := generateHMACSHA256(bodyBytes, *handlerVars.key)
		if sign != headerSign {
			http.Error(w, "Sign hashes are not equal", http.StatusBadRequest)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.Unmarshal(bodyBytes, &req)
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

	bodyBytes, err := io.ReadAll(reqBody)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	if headerSign := r.Header.Get("HashSHA256"); headerSign != "" {
		sign := generateHMACSHA256(bodyBytes, *handlerVars.key)
		if sign != headerSign {
			http.Error(w, "Sign hashes are not equal", http.StatusBadRequest)
			return
		}
	}

	err = json.Unmarshal(bodyBytes, &req)
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

	bodyBytes, err := io.ReadAll(reqBody)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	if headerSign := r.Header.Get("HashSHA256"); headerSign != "" {
		sign := generateHMACSHA256(bodyBytes, *handlerVars.key)
		if sign != headerSign {
			http.Error(w, "Sign hashes are not equal", http.StatusBadRequest)
			return
		}
	}

	err = json.Unmarshal(bodyBytes, &req)
	if err != nil {
		fmt.Println("Falls here")
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

func generateHMACSHA256(data []byte, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}
