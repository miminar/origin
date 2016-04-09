package admission

import (
	"fmt"
	"io"

	"github.com/golang/glog"

	kadmission "k8s.io/kubernetes/pkg/admission"
	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/runtime"
	limitranger "k8s.io/kubernetes/plugin/pkg/admission/limitranger"

	imageapi "github.com/openshift/origin/pkg/image/api"
)

const (
	PluginName = "ImageLimitRange"
)

func init() {
	kadmission.RegisterPlugin(PluginName, func(client clientset.Interface, config io.Reader) (kadmission.Interface, error) {
		plugin, err := NewImageLimitRangerPlugin(client, config)
		if err != nil {
			return nil, err
		}
		return plugin, nil
	})
}

// imageLimitRangerPlugin is the admission plugin.
type imageLimitRangerPlugin struct {
	*kadmission.Handler
	limitRanger kadmission.Interface
}

// imageLimitRangerPlugin implements the LimitRangerActions interface.
var _ limitranger.LimitRangerActions = &imageLimitRangerPlugin{}

// NewImageLimitRangerPlugin provides a new imageLimitRangerPlugin.
func NewImageLimitRangerPlugin(client clientset.Interface, config io.Reader) (kadmission.Interface, error) {
	plugin := &imageLimitRangerPlugin{
		Handler: kadmission.NewHandler(kadmission.Create),
	}
	limitRanger, err := limitranger.NewLimitRanger(client, plugin)
	if err != nil {
		return nil, err
	}
	plugin.limitRanger = limitRanger

	return plugin, nil
}

// Admit invokes the admission logic for checking against LimitRanges.
func (a *imageLimitRangerPlugin) Admit(attr kadmission.Attributes) error {
	if !a.SupportsAttributes(attr) {
		return nil // not applicable
	}

	return a.limitRanger.Admit(attr)
}

// SupportsAttributes is a helper that returns true if the resource is supported by the plugin.
// Implements the LimitRangerActions interface.
func (a *imageLimitRangerPlugin) SupportsAttributes(attr kadmission.Attributes) bool {
	if attr.GetSubresource() != "" {
		return false
	}

	resource := attr.GetResource()
	return resource == imageapi.Resource("imagestreammappings")
}

// SupportsLimit provides a check to see if the limitRange is applicable to image objects.
// Implements the LimitRangerActions interface.
func (a *imageLimitRangerPlugin) SupportsLimit(limitRange *kapi.LimitRange) bool {
	if limitRange == nil {
		return false
	}

	for _, limit := range limitRange.Spec.Limits {
		if limit.Type == imageapi.LimitTypeImageSize {
			return true
		}
	}
	return false
}

// Limit is the limit range implementation that checks resource against the
// image limit ranges.
// Implements the LimitRangerActions interface
func (a *imageLimitRangerPlugin) Limit(limitRange *kapi.LimitRange, kind string, obj runtime.Object) error {
	var image *imageapi.Image

	switch isObj := obj.(type) {
	case *imageapi.ImageStreamMapping:
		image = &isObj.Image
	default:
		glog.V(5).Infof("%s: received object that was not an ImageStreamMapping", PluginName)
		return nil
	}

	if err := imageapi.ImageWithMetadata(image); err != nil {
		return err
	}

	for _, limit := range limitRange.Spec.Limits {
		if err := AdmitImage(image.DockerImageMetadata.Size, limit); err != nil {
			return err
		}
	}

	return nil
}

// AdmitImage checks if the size is greater than the limit range.  Abstracted for reuse in the registry.
func AdmitImage(size int64, limit kapi.LimitRangeItem) error {
	if limit.Type != imageapi.LimitTypeImageSize {
		return nil
	}

	limitQuantity, ok := limit.Max[kapi.ResourceStorage]
	if !ok {
		return nil
	}

	imageQuantity := resource.NewQuantity(size, resource.BinarySI)
	if limitQuantity.Cmp(*imageQuantity) < 0 {
		// image size is larger than the permitted limit range max size, image is forbidden
		return fmt.Errorf("%s exceeds the maximum %s usage per %s (%s)", imageQuantity.String(), kapi.ResourceStorage, imageapi.LimitTypeImageSize, limitQuantity.String())
	}
	return nil
}
