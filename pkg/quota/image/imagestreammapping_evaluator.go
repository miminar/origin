package image

import (
	"fmt"

	"github.com/hashicorp/golang-lru"

	"k8s.io/kubernetes/pkg/admission"
	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/client/cache"
	kquota "k8s.io/kubernetes/pkg/quota"
	"k8s.io/kubernetes/pkg/quota/generic"
	"k8s.io/kubernetes/pkg/runtime"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"

	osclient "github.com/openshift/origin/pkg/client"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

const imageStreamMappingName = "Evaluator.ImageStreamMapping"

// NewImageStreamMappingEvaluator computes resource usage for ImageStreamMapping objects. This particular kind
// is a virtual resource. It depends on ImageStream usage evaluator to compute image numbers before the
// the admission can work.
func NewImageStreamMappingEvaluator(osClient osclient.Interface, imageCache cache.Store, registryAddresses *lru.Cache) kquota.Evaluator {
	computeResources := []kapi.ResourceName{
		imageapi.ResourceImages,
	}

	matchesScopeFunc := func(kapi.ResourceQuotaScope, runtime.Object) bool { return true }

	return &generic.GenericEvaluator{
		Name:                       imageStreamMappingName,
		InternalGroupKind:          kapi.Kind("ImageStreamMapping"),
		InternalOperationResources: map[admission.Operation][]kapi.ResourceName{admission.Create: computeResources},
		MatchedResourceNames:       computeResources,
		MatchesScopeFunc:           matchesScopeFunc,
		UsageFunc:                  makeImageStreamMappingAdmissionUsageFunc(osClient, imageCache, registryAddresses),
		ConstraintsFunc:            imageStreamMappingConstraintsFunc,
	}
}

// imageStreamMappingConstraintsFunc checks that given object is an image stream
func imageStreamMappingConstraintsFunc(required []kapi.ResourceName, object runtime.Object) error {
	if _, ok := object.(*imageapi.ImageStreamMapping); !ok {
		return fmt.Errorf("unexpected input object %v", object)
	}
	return nil
}

// makeImageStreamMappingAdmissionUsageFunc returns a function that computes a resource usage of image stream
// mapping objects. It is being used solely in the context of admission check for CREATE operation on
// ImageStreamMapping object.
func makeImageStreamMappingAdmissionUsageFunc(osClient osclient.Interface, imageCache cache.Store, registryAddresses *lru.Cache) generic.UsageFunc {
	return func(object runtime.Object) kapi.ResourceList {
		ism, ok := object.(*imageapi.ImageStreamMapping)
		if !ok {
			return kapi.ResourceList{}
		}

		c := NewGenericImageStreamUsageComputer(osClient, false, imageCache, registryAddresses)

		_, imagesIncrement, err := c.GetProjectImagesUsageIncrement(ism.Namespace, nil, &ism.Image)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to get project images usage increment of %q caused by an image %q: %v", ism.Namespace, ism.Image.Name, err))
			return map[kapi.ResourceName]resource.Quantity{}
		}

		return map[kapi.ResourceName]resource.Quantity{
			imageapi.ResourceImages: *imagesIncrement,
		}
	}
}
