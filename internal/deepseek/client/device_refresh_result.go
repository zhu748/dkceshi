package client

type RealDeviceIDRefreshResult struct {
	Identifier string `json:"identifier"`
	DeviceID   string `json:"device_id,omitempty"`
	Error      string `json:"error,omitempty"`
}
