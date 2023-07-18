package memstorage

import (
	"reflect"
	"sync"
	"testing"
)

func TestNewMemStorage(t *testing.T) {
	tests := []struct {
		name string
		want *MemStorage
	}{
		{
			name: "Test1",
			want: &MemStorage{Mutex: sync.RWMutex{}, Counters: make(map[string]int64), Gauges: make(map[string]float64)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewMemStorage(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewMemStorage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemStorage_GetCounter(t *testing.T) {
	type args struct {
		nameC string
	}
	tests := []struct {
		name  string
		m     *MemStorage
		args  args
		want  int64
		want1 bool
	}{
		{
			name: "Test1",
			m:    NewMemStorage(),
			args: args{
				nameC: "count",
			},
			want:  0,
			want1: false,
		},
		{
			name: "Test2",
			m: &MemStorage{
				Mutex:    sync.RWMutex{},
				Counters: map[string]int64{"count": 10},
				Gauges:   make(map[string]float64),
			},
			args: args{
				nameC: "count",
			},
			want:  10,
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.m.GetCounter(tt.args.nameC)
			if got != tt.want {
				t.Errorf("MemStorage.GetCounter() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("MemStorage.GetCounter() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestMemStorage_GetGauge(t *testing.T) {
	type args struct {
		nameG string
	}
	tests := []struct {
		name  string
		m     *MemStorage
		args  args
		want  float64
		want1 bool
	}{
		{
			name: "Test1",
			m:    NewMemStorage(),
			args: args{
				nameG: "gauge",
			},
			want:  0,
			want1: false,
		},
		{
			name: "Test2",
			m: &MemStorage{
				Mutex:    sync.RWMutex{},
				Counters: make(map[string]int64),
				Gauges:   map[string]float64{"gauge": 10.3},
			},
			args: args{
				nameG: "gauge",
			},
			want:  10.3,
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.m.GetGauge(tt.args.nameG)
			if got != tt.want {
				t.Errorf("MemStorage.GetGauge() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("MemStorage.GetGauge() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
