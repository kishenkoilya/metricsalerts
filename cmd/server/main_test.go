package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_updatePage(t *testing.T) {
	type args struct {
		res http.ResponseWriter
		req *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Test1",
			args: args{
				res: httptest.NewRecorder(),
				req: httptest.NewRequest("POST",
					"http://localhost:8080/update/gauge/Alloc/3453.13",
					http.NoBody),
			},
		},
		{
			name: "Test2",
			args: args{
				res: httptest.NewRecorder(),
				req: httptest.NewRequest("GET",
					"http://localhost:8080/update/gauge/Alloc/3453.13",
					http.NoBody),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updatePage(tt.args.res, tt.args.req)
		})
	}
}

func Test_parsePath(t *testing.T) {
	type args struct {
		pathSplit []string
		storage   *MemStorage
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "Test1",
			args: args{
				pathSplit: []string{"", "update", "counter", "asdf", "234"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 200,
		},
		{
			name: "Test2",
			args: args{
				pathSplit: []string{"", "2update", "counter", "asdf", "234"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 200,
		},
		{
			name: "Test3",
			args: args{
				pathSplit: []string{"", "update", "countere", "asdf", "234"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 400,
		},
		{
			name: "Test4",
			args: args{
				pathSplit: []string{"", "update", "gauge", "asdf", "234.1231"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 200,
		},
		{
			name: "Test5",
			args: args{
				pathSplit: []string{"", "update", "gauge", "asdf"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 404,
		},
		{
			name: "Test6",
			args: args{
				pathSplit: []string{"", "update", "gauge", "1234", "234.1231"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 404,
		},
		{
			name: "Test7",
			args: args{
				pathSplit: []string{"", "update", "counter", "asdf", "234.1231"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 400,
		},
		{
			name: "Test8",
			args: args{
				pathSplit: []string{"", "update", "gauge", "asdf", "234"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 200,
		},
		{
			name: "Test9",
			args: args{
				pathSplit: []string{"", "update", "gauge", "asdf", "234.1231.4567"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 400,
		},
		{
			name: "Test10",
			args: args{
				pathSplit: []string{"", "update", "counter", "asdf", "234.1231"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 400,
		},
		{
			name: "Test11",
			args: args{
				pathSplit: []string{"", "update", "counter", "123", "1234"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 404,
		},
		{
			name: "Test12",
			args: args{
				pathSplit: []string{"", "update", "counter"},
				storage:   &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
			want: 404,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parsePath(tt.args.pathSplit, tt.args.storage); got != tt.want {
				t.Errorf("parsePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
