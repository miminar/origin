package quota

import (
	kquota "k8s.io/kubernetes/pkg/quota"

	osclient "github.com/openshift/origin/pkg/client"
	"github.com/openshift/origin/pkg/quota/image"
)

// NewOriginQuotaRegistry returns a registry object that knows how to evaluate quota usage of OpenShift
// resources. Registry for resource quota controller cannot be used with resource quota admission plugin and
// vice versa.
func NewOriginQuotaRegistry(osClient osclient.Interface) kquota.Registry {
	return image.NewImageQuotaRegistry(osClient)
}

// NewOriginQuotaRegistryForAdmission returns a registry object that knows how to evaluate quota usage of
// OpenShift resources in a context of admission. See a package documentation of pkg/quota/image for more
// details.
func NewOriginQuotaRegistryForAdmission(osClient osclient.Interface, maxTagsPerRepo int) kquota.Registry {
	return image.NewImageQuotaRegistryForAdmission(osClient, maxTagsPerRepo)
}
