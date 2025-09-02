package utilities

// MARK: NetworkInterface
type NetworkInterface struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	IsUp      bool     `json:"is_up"`
	MTU       int      `json:"mtu"`
}
