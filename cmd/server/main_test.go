package main

import (
	"net/http"
	"sync"
	"testing"

	"github.com/kishenkoilya/metricsalerts/internal/memstorage"
)

func Test_validateValues(t *testing.T) {
	type args struct {
		mType string
		mName string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "Test1",
			args: args{
				mType: "counter",
				mName: "asdf",
			},
			want: http.StatusOK,
		},
		{
			name: "Test2",
			args: args{
				mType: "gauge",
				mName: "asdf",
			},
			want: http.StatusOK,
		},
		{
			name: "Test3",
			args: args{
				mType: "counterillo",
				mName: "asdf",
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Test4",
			args: args{
				mType: "counter",
				mName: "10",
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Test5",
			args: args{
				mType: "gauge",
				mName: "0.342",
			},
			want: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateValues(tt.args.mType, tt.args.mName); got != tt.want {
				t.Errorf("validateValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_saveValues(t *testing.T) {
	type args struct {
		storage *memstorage.MemStorage
		mType   string
		mName   string
		mVal    string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "Test1",
			args: args{
				storage: memstorage.NewMemStorage(),
				mType:   "gauge",
				mName:   "sdfs",
				mVal:    "0.23412",
			},
			want: http.StatusOK,
		},
		{
			name: "Test2",
			args: args{
				storage: memstorage.NewMemStorage(),
				mType:   "counter",
				mName:   "sdfs",
				mVal:    "23412",
			},
			want: http.StatusOK,
		},
		{
			name: "Test3",
			args: args{
				storage: memstorage.NewMemStorage(),
				mType:   "gauge",
				mName:   "sdfs",
				mVal:    "0.23sdf412",
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Test4",
			args: args{
				storage: memstorage.NewMemStorage(),
				mType:   "counter",
				mName:   "sdfs",
				mVal:    "0.23sdf412",
			},
			want: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := saveValues(tt.args.storage, tt.args.mType, tt.args.mName, tt.args.mVal); got != tt.want {
				t.Errorf("saveValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getValue(t *testing.T) {
	type args struct {
		storage *memstorage.MemStorage
		mType   string
		mName   string
	}
	tests := []struct {
		name  string
		args  args
		want  int
		want1 string
	}{
		{
			name: "Test1",
			args: args{
				storage: &memstorage.MemStorage{
					Mutex:    sync.RWMutex{},
					Counters: map[string]int64{"asdf": 10},
					Gauges:   map[string]float64{"qwer": 0.4543},
				},
				mType: "gauge",
				mName: "qwer",
			},
			want:  http.StatusOK,
			want1: "0.4543",
		},
		{
			name: "Test2",
			args: args{
				storage: &memstorage.MemStorage{
					Mutex:    sync.RWMutex{},
					Counters: map[string]int64{"asdf": 10},
					Gauges:   map[string]float64{"qwer": 0.4543},
				},
				mType: "counter",
				mName: "asdf",
			},
			want:  http.StatusOK,
			want1: "10",
		},
		{
			name: "Test3",
			args: args{
				storage: &memstorage.MemStorage{
					Mutex:    sync.RWMutex{},
					Counters: map[string]int64{"asdf": 10},
					Gauges:   map[string]float64{"qwer": 0.4543},
				},
				mType: "counter",
				mName: "asdfasfd",
			},
			want:  http.StatusNotFound,
			want1: "",
		},
		{
			name: "Test4",
			args: args{
				storage: &memstorage.MemStorage{
					Mutex:    sync.RWMutex{},
					Counters: map[string]int64{"asdf": 10},
					Gauges:   map[string]float64{"qwer": 0.4543},
				},
				mType: "gauge",
				mName: "asdfasdfasd",
			},
			want:  http.StatusNotFound,
			want1: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := getValue(tt.args.storage, tt.args.mType, tt.args.mName)
			if got != tt.want {
				t.Errorf("getValue() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("getValue() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
