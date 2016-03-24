package image

import (
	"fmt"

	"github.com/golang/glog"

	"k8s.io/kubernetes/pkg/admission"
	kapi "k8s.io/kubernetes/pkg/api"
	kerrors "k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/resource"
	kquota "k8s.io/kubernetes/pkg/quota"
	"k8s.io/kubernetes/pkg/quota/generic"
	"k8s.io/kubernetes/pkg/runtime"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"

	osclient "github.com/openshift/origin/pkg/client"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

const imageStreamTagEvaluatorName = "Evaluator.ImageStreamTag"

// NewImageStreamTagEvaluator computes resource usage of ImageStreamsTags. Its sole purpose is to handle
// UPDATE admission operations on imageStreamTags resource.
func NewImageStreamTagEvaluator(osClient osclient.Interface) kquota.Evaluator {
	computeResources := []kapi.ResourceName{
		imageapi.ResourceImageStreamImages,
	}

	matchesScopeFunc := func(kapi.ResourceQuotaScope, runtime.Object) bool { return true }
	getFuncByNamespace := func(namespace, id string) (runtime.Object, error) {
		isName, tag, err := imageapi.ParseImageStreamTagName(id)
		if err != nil {
			return nil, err
		}

		obj, err := osClient.ImageStreamTags(namespace).Get(isName, tag)
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return nil, err
			}
			obj = &imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: namespace,
					Name:      id,
				},
			}
		}
		return obj, nil
	}

	return &generic.GenericEvaluator{
		Name:                       imageStreamTagEvaluatorName,
		InternalGroupKind:          kapi.Kind("ImageStreamTag"),
		InternalOperationResources: map[admission.Operation][]kapi.ResourceName{admission.Update: computeResources},
		MatchedResourceNames:       computeResources,
		MatchesScopeFunc:           matchesScopeFunc,
		UsageFunc:                  makeImageStreamTagAdmissionUsageFunc(osClient),
		GetFuncByNamespace:         getFuncByNamespace,
		ConstraintsFunc:            imageStreamTagConstraintsFunc,
	}
}

// imageStreamTagConstraintsFunc checks that given object is an image stream tag
func imageStreamTagConstraintsFunc(required []kapi.ResourceName, object runtime.Object) error {
	if _, ok := object.(*imageapi.ImageStreamTag); !ok {
		return fmt.Errorf("unexpected input object %v", object)
	}
	return nil
}

// makeImageStreamTagAdmissionUsageFunc returns a function that computes a resource usage for given image
// stream tag during admission.
func makeImageStreamTagAdmissionUsageFunc(osClient osclient.Interface) generic.UsageFunc {
	return func(object runtime.Object) kapi.ResourceList {
		ist, ok := object.(*imageapi.ImageStreamTag)
		if !ok {
			return kapi.ResourceList{}
		}

		res := map[kapi.ResourceName]resource.Quantity{
			imageapi.ResourceImageStreamImages: *resource.NewQuantity(0, resource.BinarySI),
		}

		if ist.Tag == nil {
			glog.V(4).Infof("Nothing to tag to %s/%s", ist.Namespace, ist.Name)
			return res
		}

		isName, _, err := imageapi.ParseImageStreamTagName(ist.Name)
		if err != nil {
			glog.Error(err.Error())
			return kapi.ResourceList{}
		}

		if ist.Tag.From == nil {
			glog.V(2).Infof("from unspecified in tag reference of istag %s/%s, skipping", ist.Namespace, ist.Name)
			return res
		}

		c := NewGenericImageStreamUsageComputer(osClient, true)

		ref, err := c.GetImageReferenceForObjectReference(ist.Namespace, ist.Tag.From)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to get source docker image reference for istag %s/%s: %v", ist.Namespace, isName, err))
			return res
		}

		_, imagesIncrement, err := c.GetProjectImagesUsageIncrement(ist.Namespace, nil, ref)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to get namespace usage increment of %q with an image reference %q: %v", ist.Namespace, ref, err))
			return res
		}

		res[imageapi.ResourceImageStreamImages] = *imagesIncrement

		return res
	}
}
