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
}

// InternalImageReferenceHandler is a function passed to the computer when processing images that allows a
// caller to perform actions on image references. The handler is called on a unique image reference just once.
// Argument inSpec says whether the image reference is present in an image stream spec. The inStatus says the
// same for an image stream status.
//
// The reference can either be:
//
//  1. a docker image reference (e.g. 172.30.12.34:5000/test/is2:tag)
//  2. an image stream tag (e.g. project/isname:latest)
//  3. an image ID (e.g. sha256:2643199e5ed5047eeed22da854748ed88b3a63ba0497601ba75852f7b92d4640)
//
// The first two a can be obtained only from IS spec. Processing of IS status can generate only the 3rd
// option.
//
// The docker image reference will always be normalized such that registry url is always specified while a
// default docker namespace and tag are stripped.
type InternalImageReferenceHandler func(imageReference string, inSpec, inStatus bool) error

// NewGenericImageStreamUsageComputer returns an instance of GenericImageStreamUsageComputer.
func NewGenericImageStreamUsageComputer(osClient osclient.Interface) *GenericImageStreamUsageComputer {
	return &GenericImageStreamUsageComputer{
		osClient: osClient,
	}
}

// GetImageStreamUsage counts number of unique internally managed images occupying given image stream. It
// returns a number of unique image references found in the image stream spec not contained in
// processedSpecRefs and a number of unique image hashes contained in iS status not contained in
// processedStatusRefs. Given sets will be updated with new references found.
func (c *GenericImageStreamUsageComputer) GetImageStreamUsage(
	is *imageapi.ImageStream,
	processedSpecRefs sets.String,
	processedStatusRefs sets.String,
) (specRefs, statusRefs *resource.Quantity) {
	specRefs = resource.NewQuantity(0, resource.DecimalSI)
	statusRefs = resource.NewQuantity(0, resource.DecimalSI)

	c.ProcessImageStreamImages(is, false, func(ref string, inSpec, inStatus bool) error {
		if !processedSpecRefs.Has(ref) && inSpec {
			processedSpecRefs.Insert(ref)
			specRefs.Set(specRefs.Value() + 1)
		}
		if !processedStatusRefs.Has(ref) && inStatus {
			processedStatusRefs.Insert(ref)
			statusRefs.Set(statusRefs.Value() + 1)
		}
		return nil
	})

	return
}

// GetProjectImagesUsage returns a number of unique image references found in image stream spec and a number
// of unique image hashes found in iS status that are tagged in a given project.
func (c *GenericImageStreamUsageComputer) GetProjectImagesUsage(
	namespace string,
) (specRefs, statusRefs *resource.Quantity, err error) {
	processedSpecRefs := sets.NewString()
	processedStatusRefs := sets.NewString()

	// TODO: Remove the following call once we have one of the following:
	//  1. a shared cache on the backend for image streams and/or unique referenced images per namespace that
	//     would allow fast checks
	//  2. a wrapper admission controller on the storage object passed into ImageStreamTags,
	//	   ImageStreamImports, and ImageStreamMappings that only knew about ImageStreams and could do the
	//     accurate check
	iss, err := c.osClient.ImageStreams(namespace).List(kapi.ListOptions{})
	if err != nil {
		return
	}

	specRefs = resource.NewQuantity(0, resource.DecimalSI)
	statusRefs = resource.NewQuantity(0, resource.DecimalSI)

	for _, is := range iss.Items {
		c.ProcessImageStreamImages(&is, false, func(ref string, inSpec, inStatus bool) error {
			if !processedSpecRefs.Has(ref) && inSpec {
				processedSpecRefs.Insert(ref)
				specRefs.Set(specRefs.Value() + 1)
			}
			if !processedStatusRefs.Has(ref) && inStatus {
				processedStatusRefs.Insert(ref)
				statusRefs.Set(statusRefs.Value() + 1)
			}
			return nil
		})
	}

	return
}

// GetProjectImagesUsageIncrement computes image count in the namespace for given image stream (new or
// updated) and new image references added to IS spec and/or IS status. It returns:
//
//  1. number of unique image references across all image stream specs in the project; references of the given
//     is and image don't count unless they are tagged in other is as well
//  2. number of unique image references added either by the given is or newImageStreamTag
//  3. number of unique image hashes currently tagged in statuses of all imagestreams in the project; the image
//     and images tagged in the given is don't count unless they are tagged in other is as well
//  4. number of new image hashes being added to IS status by the is or the newImageStreamImage
//  5. an error if something goes wrong
//
// Valid newImageStreamTag is either a docker image reference or an image stream tag about to be stored
// in IS spec. newImageStreamImage can only be a hach of image about to be stored in IS status.
func (c *GenericImageStreamUsageComputer) GetProjectImagesUsageIncrement(
	namespace string,
	is *imageapi.ImageStream,
	newImageStreamTag string,
	newImageStreamImage string,
) (specRefs, specRefsIncrement, statusRefs, statusRefsIncrement *resource.Quantity, err error) {
	processedSpecRefs := sets.NewString()
	processedStatusRefs := sets.NewString()

	iss, err := c.osClient.ImageStreams(namespace).List(kapi.ListOptions{})
	if err != nil {
		return
	}

	specRefs = resource.NewQuantity(0, resource.DecimalSI)
	specRefsIncrement = resource.NewQuantity(0, resource.DecimalSI)
	statusRefs = resource.NewQuantity(0, resource.DecimalSI)
	statusRefsIncrement = resource.NewQuantity(0, resource.DecimalSI)

	for _, imageStream := range iss.Items {
		if is != nil && imageStream.Name == is.Name {
			continue
		}
		c.ProcessImageStreamImages(&imageStream, false, func(ref string, inSpec, inStatus bool) error {
			if !processedSpecRefs.Has(ref) && inSpec {
				processedSpecRefs.Insert(ref)
				specRefs.Set(specRefs.Value() + 1)
			}
			if !processedStatusRefs.Has(ref) && inStatus {
				processedStatusRefs.Insert(ref)
				statusRefs.Set(statusRefs.Value() + 1)
			}
			return nil
		})
	}

	if is != nil {
		c.ProcessImageStreamImages(is, false, func(ref string, inSpec, inStatus bool) error {
			if !processedStatusRefs.Has(ref) && inStatus {
				processedStatusRefs.Insert(ref)
				statusRefsIncrement.Set(statusRefsIncrement.Value() + 1)
			}

			if processedSpecRefs.Has(ref) || !inSpec {
				return nil
			}

			// processing a new entry to project's IS spec
			processedSpecRefs.Insert(ref)
			specRefsIncrement.Set(specRefsIncrement.Value() + 1)

			if inStatus {
				return nil
			}

			// A new tag added to spec means a new entry in the status after some time.
			// Consider it an increment of imagestreamimages if it has an ID whish is NOT already processed.
			parsed, parseErr := imageapi.ParseDockerImageReference(ref)
			if parseErr == nil && len(parsed.ID) != 0 && !processedStatusRefs.Has(parsed.ID) {
				processedStatusRefs.Insert(parsed.ID)
				statusRefsIncrement.Set(statusRefsIncrement.Value() + 1)
			}

			return nil
		})
	}

	if len(newImageStreamTag) != 0 {
		if !processedSpecRefs.Has(newImageStreamTag) {
			specRefsIncrement.Set(specRefsIncrement.Value() + 1)

			// consider the new tag an increment to isimages as well if it has an ID which is NOT already present
			parsed, parseErr := imageapi.ParseDockerImageReference(newImageStreamTag)
			if parseErr == nil && len(parsed.ID) != 0 && !processedStatusRefs.Has(parsed.ID) {
				processedStatusRefs.Insert(parsed.ID)
				statusRefsIncrement.Set(statusRefsIncrement.Value() + 1)
			}
		}
	}

	if len(newImageStreamImage) != 0 && !processedStatusRefs.Has(newImageStreamImage) {
		statusRefsIncrement.Set(statusRefsIncrement.Value() + 1)
	}

	return
}

// ProcessImageStreamImages is a utility method that calls a given handler on every image reference found in
// the given image stream. If specOnly is true, only image references found in is spec will be processed. The
// handler will be called just once for each unique image reference.
func (c *GenericImageStreamUsageComputer) ProcessImageStreamImages(
	is *imageapi.ImageStream,
	specOnly bool,
	handler InternalImageReferenceHandler,
) error {
	type sources struct{ inSpec, inStatus bool }
	var statusReferences sets.String
	imageReferences := make(map[string]*sources)

	specReferences := c.gatherImagesFromImageStreamSpec(is)
	for ref := range specReferences {
		imageReferences[ref] = &sources{inSpec: true}
	}

	if !specOnly {
		statusReferences = c.gatherImagesFromImageStreamStatus(is)
		for ref := range statusReferences {
			if s, exists := imageReferences[ref]; exists {
				s.inStatus = true
			} else {
				imageReferences[ref] = &sources{inStatus: true}
			}
		}
	}

	for ref, s := range imageReferences {
		if err := handler(ref, s.inSpec, s.inStatus); err != nil {
			return err
		}
	}
	return nil
}

// gatherImagesFromImageStreamStatus is a utility method that collects all image references found in a status
// of a given image stream.
func (c *GenericImageStreamUsageComputer) gatherImagesFromImageStreamStatus(is *imageapi.ImageStream) sets.String {
	res := sets.NewString()

	for _, history := range is.Status.Tags {
		for i := range history.Items {
			ref := history.Items[i].Image
			if len(ref) == 0 {
				continue
			}

			res.Insert(ref)
		}
	}

	return res
}

// gatherImagesFromImageStreamSpec is a utility method that collects all image references found in a spec of a
// given image stream
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

		if objRef.Kind == "ImageStreamImage" {
			if res.Namespace == "" {
				res.Namespace = objRef.Namespace
			}
			if res.Namespace == "" {
				res.Namespace = namespace
			}
			if len(res.ID) == 0 {
				return "", fmt.Errorf("missing id in ImageStreamImage reference %q", objRef.Name)
			}

		} else {
			// objRef.Kind == "DockerImage"
			res = res.DockerClientDefaults()
		}

		// docker image reference
		return res.DaemonMinimal().Exact(), nil

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
