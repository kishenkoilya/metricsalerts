package addressurl

import "testing"

func TestAddressURL_AddrCommand(t *testing.T) {
	type args struct {
		command    string
		metricType string
		metricName string
		value      string
	}
	tests := []struct {
		name string
		addr *AddressURL
		args args
		want string
	}{
		{
			name: "Test1",
			addr: &AddressURL{Protocol: "http", Address: "localhost:8080"},
			args: args{
				command:    "update",
				metricType: "gauge",
				metricName: "asdf",
			},
			want: "http://localhost:8080/update/gauge/asdf",
		},
		{
			name: "Test2",
			addr: &AddressURL{Protocol: "http", Address: "localhost:8080"},
			args: args{
				command:    "update",
				metricType: "gauge",
				metricName: "asdf",
				value:      "123.4321",
			},
			want: "http://localhost:8080/update/gauge/asdf/123.4321",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.addr.AddrCommand(tt.args.command, tt.args.metricType, tt.args.metricName, tt.args.value); got != tt.want {
				t.Errorf("AddressURL.AddrCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}
