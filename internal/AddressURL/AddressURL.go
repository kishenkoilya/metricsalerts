package AddressURL

type AddressURL struct {
	protocol string
	address  string
}

func (addr *AddressURL) AddrCommand(command, metricType, metricName, value string) string {
	if value == "" {
		return addr.protocol + "://" + addr.address + "/" + command + "/" + metricType + "/" + metricName
	}
	return addr.protocol + "://" + addr.address + "/" + command + "/" + metricType + "/" + metricName + "/" + value
}
