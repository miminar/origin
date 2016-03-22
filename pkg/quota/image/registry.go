// Package image implements evaluators of usage for images stored in an internal registry. They are supposed
// to be passed to resource quota controller and origin resource quota admission plugin. As opposed to
// kubernetes evaluators that can be used both with the controller and an admission plugin, these cannot.
// That's because they're counting a number of unique images which aren't namespaced. In order to do that they
// always need to enumerate all image streams in the project to see whether the newly tagged images are new to
// the project or not. The resource quota controller iterates over them implicitly while the admission plugin
// invokes the evaluator just once on a single object. Thus different usage implementations.
//
// To instantiate a registry for use with the resource quota controller, use NewImageRegistry. To instantiate a
// registry for use with the origin resource quota admission plugin, use NewImageRegistryForAdmission.
package image

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/hashicorp/golang-lru"

	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/quota"
	"k8s.io/kubernetes/pkg/quota/generic"

	osclient "github.com/openshift/origin/pkg/client"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

const (
	imageTTL time.Duration = time.Minute * 10
	// Maximum number of unique addresses of internal docker registry to remember.
	// TODO: get rid of caching of registry urls, use some stub value to represent internally managed images
	maxRegistryAddressesToKeep int = 16
)

// NewImageQuotaRegistry returns a registry for quota evaluation of OpenShift resources related to images in
// internal registry. It evaluates only image streams. This registry is supposed to be used with resource
// quota controller. Contained evaluators aren't usable for admission because they assume the Usage method to
// be called on all images in the project. An imageCache should be an LRU cache designed to store
// *imageapi.Image objects.
func NewImageQuotaRegistry(osClient osclient.Interface, imageCache cache.Store, registryAddresses *lru.Cache) quota.Registry {
	imageStream := NewImageStreamEvaluator(osClient, imageCache, registryAddresses)
	return &generic.GenericRegistry{
		InternalEvaluators: map[unversioned.GroupKind]quota.Evaluator{
			imageStream.GroupKind(): imageStream,
		},
	}
}

// NewImageQuotaRegistryForAdmission returns a registry for quota evaluation of OpenShift resources related to
// images in internal registry. Returned registry is supposed to be used with origin resource quota admission
// plugin. It evaluates image streams, image stream mappings and image stream tags. It cannot be passed to
// resource quota controller because contained evaluators return just usage increments. An imageCache should
// be an LRU cache designed to store *imageapi.Image objects.
func NewImageQuotaRegistryForAdmission(osClient osclient.Interface, imageCache cache.Store, registryAddresses *lru.Cache) quota.Registry {
	imageStream := NewImageStreamAdmissionEvaluator(osClient, imageCache, registryAddresses)
	imageStreamMapping := NewImageStreamMappingEvaluator(osClient, imageCache, registryAddresses)
	imageStreamTag := NewImageStreamTagEvaluator(osClient, imageCache, registryAddresses)
	return &generic.GenericRegistry{
		InternalEvaluators: map[unversioned.GroupKind]quota.Evaluator{
			imageStream.GroupKind():        imageStream,
			imageStreamMapping.GroupKind(): imageStreamMapping,
			imageStreamTag.GroupKind():     imageStreamTag,
		},
	}
}

// NewImageCache creates an expiring cache for use with NewImageRegistry and similar factory functions. It
// holds *imageapi.Image objects that are kept for a limited period of time.
func NewImageCache() cache.Store {
	return cache.NewTTLStore(func(obj interface{}) (string, error) {
		image, ok := obj.(*imageapi.Image)
		if !ok {
			return "", fmt.Errorf("expected image, got %T", obj)
		}
		return image.Name, nil
	}, imageTTL)
}

// NewRegistryAddressCache returns a cache holding addresses of internal registries. The cache can be used
// with NewImageRegistry and similar factory functions.
func NewRegistryAddressCache() *lru.Cache {
	cache, err := lru.New(maxRegistryAddressesToKeep)
	if err != nil {
		glog.Fatalf("failed to initialize cache for registry addresses: %v", err)
	}
	return cache
}
