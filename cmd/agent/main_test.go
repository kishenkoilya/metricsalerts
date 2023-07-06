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
				gaugeMetrics: []string{"Alloc", "Frees", "Sys", "GCCPUFraction"},
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
