package image

import (
	"fmt"

	"github.com/golang/glog"

	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/util/sets"

	osclient "github.com/openshift/origin/pkg/client"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

// GenericImageStreamUsageComputer allows to compute number of images stored in an internal registry in
// particular namespace.
type GenericImageStreamUsageComputer struct {
	osClient osclient.Interface
	// says whether to account for images stored in image stream's spec
	processSpec bool
}

// InternalImageReferenceHandler is a function passed to the computer when processing images that allows the
// caller to perform actions on image references. The handler is called on unique image just once. The
// reference can either be:
//
//  1. an image ID if available (e.g. sha256:2643199e5ed5047eeed22da854748ed88b3a63ba0497601ba75852f7b92d4640)
//  2. a docker image reference if available (e.g. 172.30.12.34:5000/test/is2:latest
//  3. an image stream tag (e.g. project/isname:latest)
type InternalImageReferenceHandler func(imageReference string) error

// NewGenericImageStreamUsageComputer returns an instance of GenericImageStreamUsageComputer.
// Returned object can be used just once and must be thrown away afterwards.
func NewGenericImageStreamUsageComputer(osClient osclient.Interface, processSpec bool) *GenericImageStreamUsageComputer {
	return &GenericImageStreamUsageComputer{
		osClient:    osClient,
		processSpec: processSpec,
	}
}

// GetImageStreamUsage counts number of unique internally managed images occupying given image stream. Each
// Images given in processedImages won't be taken into account. The set will be updated with new images found.
func (c *GenericImageStreamUsageComputer) GetImageStreamUsage(
	is *imageapi.ImageStream,
	processedImages sets.String,
) *resource.Quantity {
	images := resource.NewQuantity(0, resource.DecimalSI)

	c.ProcessImageStreamImages(is, func(ref string) error {
		if processedImages.Has(ref) {
			return nil
		}
		processedImages.Insert(ref)
		images.Set(images.Value() + 1)
		return nil
	})

	return images
}

// GetProjectImagesUsage returns a number of internally managed images tagged in the given namespace.
func (c *GenericImageStreamUsageComputer) GetProjectImagesUsage(namespace string) (*resource.Quantity, error) {
	processedImages := sets.NewString()

	iss, err := c.osClient.ImageStreams(namespace).List(kapi.ListOptions{})
	if err != nil {
		return nil, err
	}

	images := resource.NewQuantity(0, resource.DecimalSI)

	for _, is := range iss.Items {
		c.ProcessImageStreamImages(&is, func(ref string) error {
			if !processedImages.Has(ref) {
				processedImages.Insert(ref)
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
	imageReference string,
) (images, imagesIncrement *resource.Quantity, err error) {
	processedImages := sets.NewString()

	iss, err := c.osClient.ImageStreams(namespace).List(kapi.ListOptions{})
	if err != nil {
		return
	}

	imagesIncrement = resource.NewQuantity(0, resource.DecimalSI)

	for _, imageStream := range iss.Items {
		if is != nil && imageStream.Name == is.Name {
			continue
		}
		c.ProcessImageStreamImages(&imageStream, func(ref string) error {
			processedImages.Insert(ref)
			return nil
		})
	}

	if is != nil {
		c.ProcessImageStreamImages(is, func(ref string) error {
			if !processedImages.Has(ref) {
				processedImages.Insert(ref)
				imagesIncrement.Set(imagesIncrement.Value() + 1)
			}
			return nil
		})
	}

	if len(imageReference) != 0 && !processedImages.Has(imageReference) {
		if !processedImages.Has(imageReference) {
			ref, parseErr := imageapi.ParseDockerImageReference(imageReference)
			if parseErr != nil || len(ref.ID) == 0 || !processedImages.Has(ref.ID) {
				imagesIncrement.Set(imagesIncrement.Value() + 1)
			}
		}
	}

	images = resource.NewQuantity(int64(len(processedImages)), resource.DecimalSI)

	return
}

// ProcessImageStreamImages is a utility method that calls a given handler for every image of the given image
// stream that belongs to internal registry. It process image stream status and optionally spec.
func (c *GenericImageStreamUsageComputer) ProcessImageStreamImages(is *imageapi.ImageStream, handler InternalImageReferenceHandler) error {
	imageReferences := c.gatherImagesFromImageStreamStatus(is)

	if c.processSpec {
		specReferences := c.gatherImagesFromImageStreamSpec(is)
		for k, v := range specReferences {
			imageReferences[k] = v
		}
	}

	for ref := range imageReferences {
		if err := handler(ref); err != nil {
			return err
		}
	}
	return nil
}

// gatherImagesFromImageStreamStatus is a utility method that collects all internally managed images found in
// a status of the given image stream
func (c *GenericImageStreamUsageComputer) gatherImagesFromImageStreamStatus(is *imageapi.ImageStream) sets.String {
	res := sets.NewString()

	for _, history := range is.Status.Tags {
		for i := range history.Items {
			ref := history.Items[i].Image
			if len(ref) == 0 {
				ref = history.Items[i].DockerImageReference
			}

			res.Insert(ref)
		}
	}

	return res
}

// gatherImagesFromImageStreamSpec is a utility method that collects all internally managed images found in a
// spec of the given image stream
func (c *GenericImageStreamUsageComputer) gatherImagesFromImageStreamSpec(is *imageapi.ImageStream) sets.String {
	res := sets.NewString()

	for _, tagRef := range is.Spec.Tags {
		if tagRef.From == nil {
			continue
		}

		ref, err := c.GetImageReferenceForObjectReference(is.Namespace, tagRef.From)
		if err != nil {
			glog.V(4).Infof("could not process object reference: %v", err)
			continue
		}

		res.Insert(ref)
	}

	return res
}

// GetImageReferenceForObjectReference returns corresponding image reference for the given object
// reference representing either an image stream image or image stream tag or docker image.
func (c *GenericImageStreamUsageComputer) GetImageReferenceForObjectReference(
	namespace string,
	objRef *kapi.ObjectReference,
) (string, error) {
	switch objRef.Kind {
	case "ImageStreamImage", "DockerImage":
		res, err := imageapi.ParseDockerImageReference(objRef.Name)
		if err != nil {
			return "", err
		}
		if res.Namespace == "" {
			res.Namespace = objRef.Namespace
		}
		if res.Namespace == "" {
			res.Namespace = namespace
		}

		if len(res.ID) != 0 {
			return res.ID, nil
		}

		// docker image reference
		return res.Exact(), nil

	case "ImageStreamTag":
		isName, tag, err := imageapi.ParseImageStreamTagName(objRef.Name)
		if err != nil {
			return "", err
		}

		ns := namespace
		if len(objRef.Namespace) > 0 {
			ns = objRef.Namespace
		}

		// <namespace>/<isname>:<tag>
		return cache.MetaNamespaceKeyFunc(&kapi.ObjectMeta{
			Namespace: ns,
			Name:      imageapi.JoinImageStreamTag(isName, tag),
		})
	}

	return "", fmt.Errorf("unsupported object reference kind %s", objRef.Kind)
}
