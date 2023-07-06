package main

import (
	"net/http"
	"testing"
)

func TestMemStorage_getCounter(t *testing.T) {
	type args struct {
		nameC string
	}
	tests := []struct {
		name  string
		m     *memStorage
		args  args
		want  int64
		want1 bool
	}{
		{
			name: "Test1",
			m: &memStorage{
				map[string]int64{"asdf": 10},
				map[string]float64{"qwer": 0.4543},
			},
			args:  args{"asdf"},
			want:  10,
			want1: true,
		},
		{
			name: "Test2",
			m: &memStorage{
				map[string]int64{"asdf": 10},
				map[string]float64{"qwer": 0.4543},
			},
			args:  args{"asdff"},
			want:  0,
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.m.getCounter(tt.args.nameC)
			if got != tt.want {
				t.Errorf("memStorage.getCounter() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("memStorage.getCounter() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestMemStorage_getGauge(t *testing.T) {
	type args struct {
		nameG string
	}
	tests := []struct {
		name  string
		m     *memStorage
		args  args
		want  float64
		want1 bool
	}{
		{
			name: "Test1",
			m: &memStorage{
				map[string]int64{"asdf": 10},
				map[string]float64{"qwer": 0.4543},
			},
			args:  args{"qwer"},
			want:  0.4543,
			want1: true,
		},
		{
			name: "Test2",
			m: &memStorage{
				map[string]int64{"asdf": 10},
				map[string]float64{"qwer": 0.4543},
			},
			args:  args{"qwersd"},
			want:  0,
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := tt.m.getGauge(tt.args.nameG)
			if got != tt.want {
				t.Errorf("MemStorage.getGauge() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("MemStorage.getGauge() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

// func TestMemStorage_printAll(t *testing.T) {
// 	tests := []struct {
// 		name string
// 		m    *memStorage
// 		want string
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if got := tt.m.printAll(); got != tt.want {
// 				t.Errorf("MemStorage.printAll() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

func Test_parsePath(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name  string
		args  args
		want  int
		want1 string
		want2 string
		want3 string
	}{
		{
			name:  "Test1",
			args:  args{"/value/counter/asdf/10"},
			want:  http.StatusOK,
			want1: "counter",
			want2: "asdf",
			want3: "10",
		},
		{
			name:  "Test2",
			args:  args{"/value/counter/asdf"},
			want:  http.StatusOK,
			want1: "counter",
			want2: "asdf",
			want3: "",
		},
		{
			name:  "Test3",
			args:  args{"/value"},
			want:  http.StatusNotFound,
			want1: "",
			want2: "",
			want3: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2, got3 := parsePath(tt.args.path)
			if got != tt.want {
				t.Errorf("parsePath() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("parsePath() got1 = %v, want %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("parsePath() got2 = %v, want %v", got2, tt.want2)
			}
			if got3 != tt.want3 {
				t.Errorf("parsePath() got3 = %v, want %v", got3, tt.want3)
			}
		})
	}
}

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
		storage *memStorage
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
				storage: &memStorage{make(map[string]int64), make(map[string]float64)},
				mType:   "gauge",
				mName:   "sdfs",
				mVal:    "0.23412",
			},
			want: http.StatusOK,
		},
		{
			name: "Test2",
			args: args{
				storage: &memStorage{make(map[string]int64), make(map[string]float64)},
				mType:   "counter",
				mName:   "sdfs",
				mVal:    "23412",
			},
			want: http.StatusOK,
		},
		{
			name: "Test3",
			args: args{
				storage: &memStorage{make(map[string]int64), make(map[string]float64)},
				mType:   "gauge",
				mName:   "sdfs",
				mVal:    "0.23sdf412",
			},
			want: http.StatusBadRequest,
		},
		{
			name: "Test4",
			args: args{
				storage: &memStorage{make(map[string]int64), make(map[string]float64)},
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
		storage *memStorage
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
				storage: &memStorage{
					map[string]int64{"asdf": 10},
					map[string]float64{"qwer": 0.4543},
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
				storage: &memStorage{
					map[string]int64{"asdf": 10},
					map[string]float64{"qwer": 0.4543},
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
				storage: &memStorage{
					map[string]int64{"asdf": 10},
					map[string]float64{"qwer": 0.4543},
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
				storage: &memStorage{
					map[string]int64{"asdf": 10},
					map[string]float64{"qwer": 0.4543},
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
