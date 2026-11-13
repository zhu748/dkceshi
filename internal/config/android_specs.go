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
	Brand    string `json:"brand"`
	APILevel string `json:"api_level"`
}

var androidDeviceSpecs = func() []androidDeviceSpec {
	var specs []androidDeviceSpec
	if err := json.Unmarshal(androidDeviceSpecsJSON, &specs); err != nil {
		panic(err)
	}
	return specs
}()

// androidBrandByModel keeps Build.BRAND values exact for pool models. Entries
// without a brand fall back to the prefix rules in AndroidBrandForModel.
var androidBrandByModel = func() map[string]string {
	m := make(map[string]string, len(androidDeviceSpecs))
	for _, spec := range androidDeviceSpecs {
		if spec.Brand != "" {
			m[spec.Model] = spec.Brand
		}
	}
	return m
}()
