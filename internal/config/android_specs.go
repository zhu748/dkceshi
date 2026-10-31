package config

import (
	_ "embed"
	"encoding/json"
)

//go:embed android_device_specs.json
var androidDeviceSpecsJSON []byte

// This embedded source is also read by the Node SDK profile builder. Runtime
// bundles include the JSON file, so model/version/hardware mappings cannot drift.
type androidDeviceSpec struct {
	Model    string `json:"model"`
	APILevel string `json:"api_level"`
}

var androidVivoSpecs = func() []androidDeviceSpec {
	var specs []androidDeviceSpec
	if err := json.Unmarshal(androidDeviceSpecsJSON, &specs); err != nil {
		panic(err)
	}
	return specs
}()
