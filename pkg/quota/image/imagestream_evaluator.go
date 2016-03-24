package image

import (
	"fmt"

	"k8s.io/kubernetes/pkg/admission"
	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	kquota "k8s.io/kubernetes/pkg/quota"
	"k8s.io/kubernetes/pkg/quota/generic"
	"k8s.io/kubernetes/pkg/runtime"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/util/sets"

	osclient "github.com/openshift/origin/pkg/client"
	imageapi "github.com/openshift/origin/pkg/image/api"
	quotautil "github.com/openshift/origin/pkg/quota/util"
)

const (
	imageStreamEvaluatorName          = "Evaluator.ImageStream.Controller"
	imageStreamAdmissionEvaluatorName = "Evaluator.ImageStream.Admission"
)

// NewImageStreamEvaluator computes resource usage of ImageStreams. Instantiating this is necessary for
// resource quota admission controller to properly work on image stream related objects.
func NewImageStreamEvaluator(osClient osclient.Interface) kquota.Evaluator {
	listFuncByNamespace := func(namespace string, options kapi.ListOptions) (runtime.Object, error) {
		return osClient.ImageStreams(namespace).List(options)
	}

	evaluator := NewImageStreamAdmissionEvaluator(osClient)
	genericEvaluator := evaluator.(*generic.GenericEvaluator)
	genericEvaluator.Name = imageStreamEvaluatorName
	// This evaluator isn't meant to be used for admission. Therefore let's clear supported admission
	// operations.
	genericEvaluator.InternalOperationResources = nil
	// Resource quota controller will need to enumerate image streams in the namespace which is not needed for
	// admission.
	genericEvaluator.ListFuncByNamespace = listFuncByNamespace

	return quotautil.NewSharedContextEvaluator(
		genericEvaluator,
		makeImageStreamUsageComputerFactory(osClient))
}

// NewImageStreamAdmissionEvaluator computes resource usage of ImageStreams in the context of admission
// plugin.
func NewImageStreamAdmissionEvaluator(osClient osclient.Interface) kquota.Evaluator {
	computeResources := []kapi.ResourceName{
		imageapi.ResourceImageStreamImages,
	}

	matchesScopeFunc := func(kapi.ResourceQuotaScope, runtime.Object) bool { return true }
	getFuncByNamespace := func(namespace, name string) (runtime.Object, error) {
		return osClient.ImageStreams(namespace).Get(name)
	}

	return &generic.GenericEvaluator{
		Name:              imageStreamAdmissionEvaluatorName,
		InternalGroupKind: kapi.Kind("ImageStream"),
		InternalOperationResources: map[admission.Operation][]kapi.ResourceName{
			admission.Create: computeResources,
			admission.Update: computeResources,
		},
		MatchedResourceNames: computeResources,
		MatchesScopeFunc:     matchesScopeFunc,
		UsageFunc:            makeImageStreamAdmissionUsageFunc(osClient),
		GetFuncByNamespace:   getFuncByNamespace,
		ConstraintsFunc:      imageStreamConstraintsFunc,
	}
}

// imageStreamConstraintsFunc checks that given object is an image stream
func imageStreamConstraintsFunc(required []kapi.ResourceName, object runtime.Object) error {
	if _, ok := object.(*imageapi.ImageStream); !ok {
		return fmt.Errorf("unexpected input object %v", object)
	}
	return nil
}

// makeImageStreamUsageComputerFactory returns an object used during computation of image quota across all
// repositories in a namespace.
func makeImageStreamUsageComputerFactory(osClient osclient.Interface) quotautil.UsageComputerFactory {
	return func() quotautil.UsageComputer {
		return &imageStreamUsageComputer{
			GenericImageStreamUsageComputer: *NewGenericImageStreamUsageComputer(osClient, true),
			processedImages:                 sets.NewString(),
		}
	}
}

// imageStreamUsageComputer is a context object for use in SharedContextEvaluator.
type imageStreamUsageComputer struct {
	GenericImageStreamUsageComputer
	processedImages sets.String
}

// Usage returns a usage for an image stream.
func (c *imageStreamUsageComputer) Usage(object runtime.Object) kapi.ResourceList {
	is, ok := object.(*imageapi.ImageStream)
	if !ok {
		return kapi.ResourceList{}
	}

	images := c.GetImageStreamUsage(is, c.processedImages)
	return kapi.ResourceList{
		imageapi.ResourceImageStreamImages: *images,
	}
}

// makeImageStreamAdmissionUsageFunc retuns a function for computing a usage of an image stream.
func makeImageStreamAdmissionUsageFunc(osClient osclient.Interface) generic.UsageFunc {
	return func(object runtime.Object) kapi.ResourceList {
		is, ok := object.(*imageapi.ImageStream)
		if !ok {
			return kapi.ResourceList{}
		}

		c := NewGenericImageStreamUsageComputer(osClient, true)

		_, imagesIncrement, err := c.GetProjectImagesUsageIncrement(is.Namespace, is, "")
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to compute project images usage increment in namespace %q: %v", is.Namespace, err))
			return kapi.ResourceList{}
		}

		return map[kapi.ResourceName]resource.Quantity{
			imageapi.ResourceImageStreamImages: *imagesIncrement,
		}
	}
}
