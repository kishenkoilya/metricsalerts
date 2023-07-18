package addressurl

type AddressURL struct {
	Protocol string
	Address  string
}

func (addr *AddressURL) AddrCommand(command, metricType, metricName, value string) string {
	if value == "" {
		return addr.Protocol + "://" + addr.Address + "/" + command + "/" + metricType + "/" + metricName
	}
	return addr.Protocol + "://" + addr.Address + "/" + command + "/" + metricType + "/" + metricName + "/" + value
}
