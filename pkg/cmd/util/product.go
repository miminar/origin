package util

import (
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/kardianos/osext"
)

const (
	ProductOrigin    = `Origin`
	ProductOpenShift = `OpenShift`
	ProductAtomic    = `Atomic`
)

// GetProductName chooses appropriate product for a real path of current
// executable.
func GetProductName() string {
	path, err := osext.Executable()
	if err != nil {
		glog.Warning("Failed to get absolute binary path: %v", err)
		path = os.Args[0]
	}
	name := filepath.Base(path)
	for {
		switch name {
		case "openshift":
			return ProductOpenShift
		case "atomic-enterprise":
			return ProductAtomic
		default:
			return ProductOrigin
		}
	}
}
