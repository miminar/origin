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

const imageStreamImportName = "Evaluator.ImageStreamImport"

// NewImageStreamImportEvaluator computes resource usage for ImageStreamImport objects. This particular kind
// is a virtual resource. It depends on ImageStream usage evaluator to compute image numbers before the
// the admission can work.
func NewImageStreamImportEvaluator(osClient osclient.Interface, maximumTagsPerRepo int) kquota.Evaluator {
	computeResources := []kapi.ResourceName{
		imageapi.ResourceImageStreamTags,
		imageapi.ResourceImageStreamImages,
	}

	matchesScopeFunc := func(kapi.ResourceQuotaScope, runtime.Object) bool { return true }

	return &generic.GenericEvaluator{
		Name:                       imageStreamImportName,
		InternalGroupKind:          kapi.Kind("ImageStreamImport"),
		InternalOperationResources: map[admission.Operation][]kapi.ResourceName{admission.Create: computeResources},
		MatchedResourceNames:       computeResources,
		MatchesScopeFunc:           matchesScopeFunc,
		UsageFunc:                  makeImageStreamImportAdmissionUsageFunc(osClient, maximumTagsPerRepo),
		ConstraintsFunc:            imageStreamImportConstraintsFunc,
	}
}

// imageStreamImportConstraintsFunc checks that given object is an image stream import.
func imageStreamImportConstraintsFunc(required []kapi.ResourceName, object runtime.Object) error {
	if _, ok := object.(*imageapi.ImageStreamImport); !ok {
		return fmt.Errorf("unexpected input object %v", object)
	}
	return nil
}

// imageStreamImportUsageComputer computes resource usage of image stream import objects.
type imageStreamImportUsageComputer struct {
	GenericImageStreamUsageComputer
	// we assume that import of a repository will result in a maximal number of tags possible specified by
	// this value
	maximumTagsPerRepo  int
	processedSpecRefs   sets.String
	processedStatusRefs sets.String
}

// makeImageStreamImportAdmissionUsageFunc retuns a function for computing a usage of an image stream import.
func makeImageStreamImportAdmissionUsageFunc(osClient osclient.Interface, maximumTagsPerRepo int) generic.UsageFunc {
	f := makeImageStreamImportUsageComputerFactory(osClient, maximumTagsPerRepo)
	return func(object runtime.Object) kapi.ResourceList {
		return f().Usage(object)
	}
}

// makeImageStreamImportUsageComputerFactory returns an object used during computation of image quota across all
// repositories in a namespace.
func makeImageStreamImportUsageComputerFactory(osClient osclient.Interface, maximumTagsPerRepo int) quotautil.UsageComputerFactory {
	return func() quotautil.UsageComputer {
		return &imageStreamImportUsageComputer{
			GenericImageStreamUsageComputer: *NewGenericImageStreamUsageComputer(osClient),
			maximumTagsPerRepo:              maximumTagsPerRepo,
			processedSpecRefs:               sets.NewString(),
			processedStatusRefs:             sets.NewString(),
		}
	}
}

// makeImageStreamImportAdmissionUsageFunc returns a function that computes a resource usage of image stream
// import objects. It is being used solely in the context of admission check for CREATE operation on
// ImageStreamImport kind.
func (c *imageStreamImportUsageComputer) Usage(object runtime.Object) kapi.ResourceList {
	isi, ok := object.(*imageapi.ImageStreamImport)
	if !ok {
		return kapi.ResourceList{}
	}

	usage := map[kapi.ResourceName]resource.Quantity{
		imageapi.ResourceImageStreamTags:   *resource.NewQuantity(0, resource.DecimalSI),
		imageapi.ResourceImageStreamImages: *resource.NewQuantity(0, resource.DecimalSI),
	}

	if !isi.Spec.Import || (len(isi.Spec.Images) == 0 && isi.Spec.Repository == nil) {
		return usage
	}

	// TODO: Remove the following call once we have one of the following:
	//  1. a shared cache on the backend for image streams and/or unique referenced images per namespace that
	//     would allow fast checks
	//  2. a wrapper admission controller on the storage object passed into ImageStreamTags,
	//	   ImageStreamImports, and ImageStreamMappings that only knew about ImageStreams and could do the
	//     accurate check
	iss, err := c.osClient.ImageStreams(isi.Namespace).List(kapi.ListOptions{})
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to list image streams: %v", err))
		return usage
	}

	for _, imageStream := range iss.Items {
		c.ProcessImageStreamImages(&imageStream, false, func(ref string, inSpec, inStatus bool) error {
			if !c.processedSpecRefs.Has(ref) && inSpec {
				c.processedSpecRefs.Insert(ref)
			}
			if !c.processedStatusRefs.Has(ref) && inStatus {
				c.processedStatusRefs.Insert(ref)
			}
			return nil
		})
	}

	// first process and remember individual images
	imagesUsage := c.getImagesUsageIncrement(isi)
	// second process a repository whose tags shall we import
	repositoryUsage := c.getRepositoryUsageIncrement(isi)

	return kquota.Add(imagesUsage, repositoryUsage)
}

func (c *imageStreamImportUsageComputer) getImagesUsageIncrement(isi *imageapi.ImageStreamImport) kapi.ResourceList {
	specRefsIncrement := resource.NewQuantity(0, resource.DecimalSI)
	statusRefsIncrement := resource.NewQuantity(0, resource.DecimalSI)

	for _, imageSpec := range isi.Spec.Images {
		if imageSpec.From.Kind != "DockerImage" {
			continue
		}
		ref, err := c.GetImageReferenceForObjectReference(isi.Namespace, &imageSpec.From)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to resolve image spec.From of isimport %s/%s: %v", isi.Namespace, isi.Name, err))
			continue
		}
		if !c.processedSpecRefs.Has(ref) {
			c.processedSpecRefs.Insert(ref)
			specRefsIncrement.Set(specRefsIncrement.Value() + 1)

			// consider the new tag an increment to isimages as well unless its ID is already present
			parsed, parseErr := imageapi.ParseDockerImageReference(ref)
			if parseErr != nil || len(parsed.ID) == 0 || !c.processedStatusRefs.Has(parsed.ID) {
				c.processedStatusRefs.Insert(parsed.ID)
				statusRefsIncrement.Set(statusRefsIncrement.Value() + 1)
			}
		}

	}

	return map[kapi.ResourceName]resource.Quantity{
		imageapi.ResourceImageStreamTags:   *specRefsIncrement,
		imageapi.ResourceImageStreamImages: *statusRefsIncrement,
	}
}

// getRepositoryUsageIncrement returns a usage increment for ImageStreamImport.Spec.Repository. The repository
// causes image importer to import all the repository tags in an image stream. We don't attempt to resolve
// number of tags here. We just expect the worst case scenario where the max limit of import tags per
// repository is reached and decrease it by the number of image references pointing to the repository that are
// already tagged in a project.
func (c *imageStreamImportUsageComputer) getRepositoryUsageIncrement(isi *imageapi.ImageStreamImport) kapi.ResourceList {
	specRefsIncrement := resource.NewQuantity(0, resource.DecimalSI)
	statusRefsIncrement := resource.NewQuantity(0, resource.DecimalSI)
	usage := map[kapi.ResourceName]resource.Quantity{
		imageapi.ResourceImageStreamTags:   *specRefsIncrement,
		imageapi.ResourceImageStreamImages: *statusRefsIncrement,
	}

	if isi.Spec.Repository == nil || isi.Spec.Repository.From.Kind != "DockerImage" {
		return usage
	}

	repoRef, err := c.GetImageReferenceForObjectReference(isi.Namespace, &isi.Spec.Repository.From)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to resolve Spec.Repository.From of isimport %s/%s: %v", isi.Namespace, isi.Name, err))
		return usage
	}

	// we cannot be sure, how many references will be added to the is, let's assume the worst case -
	// we're going to reach the maximal limit
	usageIncrement := c.maximumTagsPerRepo

	// now let's decrease the istags increments for all the repository references already tagged in specs
	// of all image streams in the project
	for ref := range c.processedSpecRefs {
		if usageIncrement <= 0 {
			break
		}
		ref, parseErr := imageapi.ParseDockerImageReference(ref)
		if parseErr != nil || len(ref.ID) != 0 {
			continue
		}
		ref.Tag = ""
		if ref.DaemonMinimal().Exact() == repoRef {
			usageIncrement -= 1
		}
	}

	q := resource.NewQuantity(int64(usageIncrement), resource.DecimalSI)
	usage[imageapi.ResourceImageStreamTags] = *q
	// a decrease of tags added to spec means an equivalent decrease of references added to status
	usage[imageapi.ResourceImageStreamImages] = *q

	return usage
}
