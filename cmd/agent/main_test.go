package main

import (
	"runtime"
	"testing"
)

func Test_updateMetrics(t *testing.T) {
	type args struct {
		m            *runtime.MemStats
		gaugeMetrics []string
		storage      *MemStorage
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "normalTest",
			args: args{
				m:            new(runtime.MemStats),
				gaugeMetrics: []string{"Alloc", "Frees", "Sys", "GCCPUFraction", "asdsdfsdf"},
				storage:      &MemStorage{counters: make(map[string]int64), gauges: make(map[string]float64)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateMetrics(tt.args.m, tt.args.gaugeMetrics, tt.args.storage)
		})
	}
}

func Test_sendMetrics(t *testing.T) {
	type args struct {
		storage *MemStorage
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "3 metrics",
			args: args{
				storage: &MemStorage{
					counters: map[string]int64{"PollCount": 3452},
					gauges:   map[string]float64{"RandomValue": 234.1},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sendMetrics(tt.args.storage)
		})
	}
}
