package origin

import (
	configapi "github.com/openshift/origin/pkg/cmd/server/api"
)

// AssetConfig defines the required parameters for starting the OpenShift master
type AssetConfig struct {
	Options configapi.AssetConfig

	WebConsoleDisabled bool
}

// BuildAssetConfig returns a new AssetConfig
func BuildAssetConfig(options configapi.MasterConfig) (*AssetConfig, error) {
	return &AssetConfig{
		Options:            *options.AssetConfig,
		WebConsoleDisabled: options.DisabledFeatures.Has(configapi.FeatureWebConsole),
	}, nil
}
