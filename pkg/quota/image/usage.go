package image

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/hashicorp/golang-lru"

	kapi "k8s.io/kubernetes/pkg/api"
	kerrors "k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/client/cache"
	ktypes "k8s.io/kubernetes/pkg/types"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/util/sets"

	osclient "github.com/openshift/origin/pkg/client"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

// ImageByName maps a name to a corresponding image object
type ImageByName map[string]*imageapi.Image

// GenericImageStreamUsageComputer allows to compute number of images stored
// in an internal registry in particular namespace.
type GenericImageStreamUsageComputer struct {
	osClient osclient.Interface
	// says whether to account for images stored in image stream's spec
	processSpec bool
	// maps image name to an image object. It holds only images stored in the registry to avoid multiple
	// fetches of the same object.
	imageCache cache.Store
	// maps image stream name prefixed by its namespace to the image stream object
	imageStreamCache cache.Store
	// is a cache of addresses referencing internal docker registry
	registryAddresses *lru.Cache
}

// InternalImageHandler is a function passed to the computer when processing images that allows the caller to
// perform actions on internally managed images. The handler is called on unique image just once.
type InternalImageHandler func(image *imageapi.Image) error

// NewGenericImageStreamUsageComputer returns an instance of GenericImageStreamUsageComputer.
// Returned object can be used just once and must be thrown away afterwards.
func NewGenericImageStreamUsageComputer(osClient osclient.Interface, processSpec bool, imageCache cache.Store, registryAddresses *lru.Cache) *GenericImageStreamUsageComputer {
	return &GenericImageStreamUsageComputer{
		osClient:          osClient,
		processSpec:       processSpec,
		imageCache:        imageCache,
		imageStreamCache:  cache.NewStore(cache.MetaNamespaceKeyFunc),
		registryAddresses: registryAddresses,
	}
}

// GetImageStreamUsage counts number of unique internally managed images occupying given image stream. Each
// Images given in processedImages won't be taken into account. The set will be updated with new images found.
func (c *GenericImageStreamUsageComputer) GetImageStreamUsage(
	is *imageapi.ImageStream,
	processedImages sets.String,
) *resource.Quantity {
	images := resource.NewQuantity(0, resource.DecimalSI)

	c.processImageStreamImages(is, func(img *imageapi.Image) error {
		if processedImages.Has(img.Name) {
			return nil
		}
		processedImages.Insert(img.Name)
		images.Set(images.Value() + 1)
		return nil
	})

	return images
}

// GetProjectImagesUsage returns a number of internally managed images tagged in the given namespace.
func (c *GenericImageStreamUsageComputer) GetProjectImagesUsage(namespace string) (*resource.Quantity, error) {
	processedImages := sets.NewString()

	iss, err := c.listImageStreams(namespace)
	if err != nil {
		return nil, err
	}

	images := resource.NewQuantity(0, resource.DecimalSI)

	for _, is := range iss.Items {
		c.processImageStreamImages(&is, func(img *imageapi.Image) error {
			if !processedImages.Has(img.Name) {
				processedImages.Insert(img.Name)
				images.Set(images.Value() + 1)
			}
			return nil
		})
	}

	return images, err
}

// GetProjectImagesUsageIncrement computes image count in the namespace for given image
// stream (new or updated) and new image. It returns:
//
//  1. number of images currently tagged in the namespace; the image and images tagged in the given is don't
//     count unless they are tagged in other is as well
//  2. number of new internally managed images referenced either by the is or by the image
//  3. an error if something goes wrong
func (c *GenericImageStreamUsageComputer) GetProjectImagesUsageIncrement(
	namespace string,
	is *imageapi.ImageStream,
	image *imageapi.Image,
) (images, imagesIncrement *resource.Quantity, err error) {
	processedImages := sets.NewString()

	iss, err := c.listImageStreams(namespace)
	if err != nil {
		return
	}

	imagesIncrement = resource.NewQuantity(0, resource.DecimalSI)

	for _, imageStream := range iss.Items {
		if is != nil && imageStream.Name == is.Name {
			continue
		}
		c.processImageStreamImages(&imageStream, func(img *imageapi.Image) error {
			processedImages.Insert(img.Name)
			return nil
		})
	}

	if is != nil {
		c.processImageStreamImages(is, func(img *imageapi.Image) error {
			if !processedImages.Has(img.Name) {
				processedImages.Insert(img.Name)
				imagesIncrement.Set(imagesIncrement.Value() + 1)
			}
			return nil
		})
	}

	if image != nil && !processedImages.Has(image.Name) {
		if value, ok := image.Annotations[imageapi.ManagedByOpenShiftAnnotation]; ok && value == "true" {
			if !processedImages.Has(image.Name) {
				imagesIncrement.Set(imagesIncrement.Value() + 1)
			}
		}
	}

	images = resource.NewQuantity(int64(len(processedImages)), resource.DecimalSI)

	return
}

// processImageStreamImages is a utility method that calls a given handler for every image of the given image
// stream that belongs to internal registry. It process image stream status and optionally spec.
func (c *GenericImageStreamUsageComputer) processImageStreamImages(is *imageapi.ImageStream, handler InternalImageHandler) error {
	imageByName := c.gatherImagesFromImageStreamStatus(is)

	if c.processSpec {
		specImageMap := c.gatherImagesFromImageStreamSpec(is)
		for k, v := range specImageMap {
			imageByName[k] = v
		}
	}

	for _, image := range imageByName {
		if err := handler(image); err != nil {
			return err
		}
	}
	return nil
}

// gatherImagesFromImageStreamStatus is a utility method that collects all internally managed images found in
// a status of the given image stream
func (c *GenericImageStreamUsageComputer) gatherImagesFromImageStreamStatus(is *imageapi.ImageStream) ImageByName {
	res := make(ImageByName)

	for _, history := range is.Status.Tags {
		for i := range history.Items {
			imageName := history.Items[i].Image
			if len(history.Items[i].DockerImageReference) == 0 || len(imageName) == 0 {
				continue
			}

			img, err := c.getImage(imageName)
			if err != nil {
				if !kerrors.IsNotFound(err) {
					utilruntime.HandleError(fmt.Errorf("failed to get image %s: %v", imageName, err))
				}
				continue
			}

			if value, ok := img.Annotations[imageapi.ManagedByOpenShiftAnnotation]; !ok || value != "true" {
				glog.V(5).Infof("image %q with DockerImageReference %q belongs to an external registry - skipping", img.Name, img.DockerImageReference)
				continue
			}

			c.cacheInternalRegistryName(img.DockerImageReference)

			res[img.Name] = img
		}
	}

	return res
}

// gatherImagesFromImageStreamSpec is a utility method that collects all internally managed images found in a
// spec of the given image stream
func (c *GenericImageStreamUsageComputer) gatherImagesFromImageStreamSpec(is *imageapi.ImageStream) ImageByName {
	res := make(ImageByName)

	for tag, tagRef := range is.Spec.Tags {
		if tagRef.From == nil {
			continue
		}

		ref, err := c.getImageReferenceForObjectReference(is.Namespace, tagRef.From)
		if err != nil {
			glog.V(4).Infof("could not process object reference: %v", err)
			continue
		}

		var img *imageapi.Image

		imageObject, exists, err := c.imageCache.GetByKey(ref.ID)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to lookup image by id %s: %v", ref.ID, err))
			continue
		}
		img, isAnImage := imageObject.(*imageapi.Image)
		if !exists || !isAnImage {
			imageID := ref.ID
			if len(imageID) == 0 {
				tag = ref.Tag
				if len(tag) == 0 {
					tag = imageapi.DefaultImageTag
				}
				event := imageapi.LatestTaggedImage(is, tag)
				if event == nil || len(event.Image) == 0 {
					glog.V(4).Infof("failed to resolve istag %s", imageapi.JoinImageStreamTag(is.Name, tag))
					continue
				}
				imageID = event.Image
			}

			img, err = c.getImage(ref.ID)
			if err != nil {
				if !kerrors.IsNotFound(err) {
					utilruntime.HandleError(fmt.Errorf("failed to get image %s: %v", ref.ID, err))
				}
				continue
			}
		}

		if value, ok := img.Annotations[imageapi.ManagedByOpenShiftAnnotation]; !ok || value != "true" {
			glog.V(4).Infof("image %q with DockerImageReference %q belongs to an external registry - skipping", img.Name, img.DockerImageReference)
			continue
		}

		c.cacheInternalRegistryName(img.DockerImageReference)

		res[img.Name] = img
	}

	return res
}

// getImageReferenceForObjectReference returns corresponding docker image reference for the given object
// reference representing either an image stream image or image stream tag or docker image.
func (c *GenericImageStreamUsageComputer) getImageReferenceForObjectReference(
	namespace string,
	objRef *kapi.ObjectReference,
) (imageapi.DockerImageReference, error) {
	switch objRef.Kind {
	case "ImageStreamImage":
		res, err := imageapi.ParseDockerImageReference(objRef.Name)
		if err != nil {
			return imageapi.DockerImageReference{}, err
		}
		if res.Namespace == "" {
			res.Namespace = objRef.Namespace
		}
		if res.Namespace == "" {
			res.Namespace = namespace
		}
		return res, nil

	case "ImageStreamTag":
		// This is really fishy. An admission check can be easily worked around by setting a tag reference
		// to an ImageStreamTag with no or small image and then tagging a large image to the source tag.
		// TODO: Shall we refuse an ImageStreamTag set in the spec if the quota is set?
		isName, tag, err := imageapi.ParseImageStreamTagName(objRef.Name)
		if err != nil {
			return imageapi.DockerImageReference{}, err
		}

		ns := namespace
		if len(objRef.Namespace) > 0 {
			ns = objRef.Namespace
		}

		is, err := c.getImageStream(ns, isName)
		if err != nil {
			return imageapi.DockerImageReference{}, fmt.Errorf("failed to get imageStream for ImageStreamTag %s/%s: %v", ns, objRef.Name, err)
		}

		event := imageapi.LatestTaggedImage(is, tag)
		if event == nil || len(event.DockerImageReference) == 0 {
			return imageapi.DockerImageReference{}, fmt.Errorf("%q is not currently pointing to an image, cannot use it as the source of a tag", objRef.Name)
		}
		return imageapi.ParseDockerImageReference(event.DockerImageReference)

	case "DockerImage":
		managedByOS, ref := c.imageReferenceBelongsToInternalRegistry(objRef.Name)
		if !managedByOS {
			return imageapi.DockerImageReference{}, fmt.Errorf("DockerImage %s does not belong to internal registry", objRef.Name)
		}
		return ref, nil
	}

	return imageapi.DockerImageReference{}, fmt.Errorf("unsupported object reference kind %s", objRef.Kind)
}

// getImageStream gets an image stream object from etcd and caches the result for the following queries.
func (c *GenericImageStreamUsageComputer) getImageStream(namespace, name string) (*imageapi.ImageStream, error) {
	isObject, exists, _ := c.imageStreamCache.GetByKey(ktypes.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}.String())
	if exists {
		converted, ok := isObject.(*imageapi.ImageStream)
		if !ok {
			return nil, fmt.Errorf("unexpected object %T stored in imageStreamCache", isObject)
		}
		return converted, nil
	}
	is, err := c.osClient.ImageStreams(namespace).Get(name)
	if err == nil {
		c.imageStreamCache.Add(is)
	}
	return is, err
}

// getImage gets image object from etcd and caches the result for the following queries.
func (c *GenericImageStreamUsageComputer) getImage(name string) (*imageapi.Image, error) {
	imageObject, exists, _ := c.imageCache.GetByKey(name)
	if exists {
		converted, ok := imageObject.(*imageapi.Image)
		if !ok {
			return nil, fmt.Errorf("unexpected object %T stored in imageCache", imageObject)
		}
		return converted, nil
	}
	image, err := c.osClient.Images().Get(name)
	if err == nil {
		c.imageCache.Add(image)
	}
	return image, err
}

// listImageStreams returns a list of image streams of the given namespace and caches them for later access.
func (c *GenericImageStreamUsageComputer) listImageStreams(namespace string) (*imageapi.ImageStreamList, error) {
	iss, err := c.osClient.ImageStreams(namespace).List(kapi.ListOptions{})
	if err == nil {
		for _, is := range iss.Items {
			c.imageStreamCache.Add(&is)
		}
	}
	return iss, err
}

// cacheInternalRegistryName caches registry name of the given docker image reference of an image stored in an
// internal registry.
func (c *GenericImageStreamUsageComputer) cacheInternalRegistryName(dockerImageReference string) {
	ref, err := imageapi.ParseDockerImageReference(dockerImageReference)
	if err == nil && len(ref.Registry) > 0 {
		c.registryAddresses.Add(ref.Registry, struct{}{})
	}
}

// imageReferenceBelongsToInternalRegistry returns true if the given docker image reference refers to an
// image in an internal registry.
func (c *GenericImageStreamUsageComputer) imageReferenceBelongsToInternalRegistry(dockerImageReference string) (bool, imageapi.DockerImageReference) {
	ref, err := imageapi.ParseDockerImageReference(dockerImageReference)
	if err != nil || len(ref.Registry) == 0 || len(ref.Namespace) == 0 || len(ref.Name) == 0 {
		return false, ref
	}
	return c.registryAddresses.Contains(ref.Registry), ref
}
