package main

import (
	"runtime"
	"sync"
	"testing"

	"github.com/kishenkoilya/metricsalerts/internal/memStorage"
)

func Test_updateMetrics(t *testing.T) {
	type args struct {
		m            *runtime.MemStats
		gaugeMetrics []string
		storage      *memStorage.MemStorage
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
				storage:      &memStorage.MemStorage{Mutex: sync.RWMutex{}, Counters: make(map[string]int64), Gauges: make(map[string]float64)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateMetrics(tt.args.m, tt.args.gaugeMetrics, tt.args.storage)
		})
	}
}
