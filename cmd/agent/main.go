package main

import (
	"fmt"
	"net/http"
	"runtime"
)

func main() {
	resp, err := http.Post("http://localhost:8080/update/counter/NumCPU/"+fmt.Sprint(runtime.NumCPU()), "text/plain", http.NoBody)
	if err != nil {
		panic(err)
	} else {
		fmt.Println(resp.Proto + " " + resp.Status)
		for k, v := range resp.Header {
			fmt.Print(k + ": ")
			for _, s := range v {
				fmt.Print(fmt.Sprint(s))
			}
			fmt.Print("\n")
		}
	}
}
