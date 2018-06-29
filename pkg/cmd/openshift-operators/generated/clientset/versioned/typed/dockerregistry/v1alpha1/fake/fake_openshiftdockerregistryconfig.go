// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/openshift/origin/pkg/cmd/openshift-operators/apis/dockerregistry/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeOpenShiftDockerRegistryConfigs implements OpenShiftDockerRegistryConfigInterface
type FakeOpenShiftDockerRegistryConfigs struct {
	Fake *FakeDockerregistryV1alpha1
}

var openshiftdockerregistryconfigsResource = schema.GroupVersionResource{Group: "dockerregistry", Version: "v1alpha1", Resource: "openshiftdockerregistryconfigs"}

var openshiftdockerregistryconfigsKind = schema.GroupVersionKind{Group: "dockerregistry", Version: "v1alpha1", Kind: "OpenShiftDockerRegistryConfig"}

// Get takes name of the openShiftDockerRegistryConfig, and returns the corresponding openShiftDockerRegistryConfig object, and an error if there is any.
func (c *FakeOpenShiftDockerRegistryConfigs) Get(name string, options v1.GetOptions) (result *v1alpha1.OpenShiftDockerRegistryConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(openshiftdockerregistryconfigsResource, name), &v1alpha1.OpenShiftDockerRegistryConfig{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.OpenShiftDockerRegistryConfig), err
}

// List takes label and field selectors, and returns the list of OpenShiftDockerRegistryConfigs that match those selectors.
func (c *FakeOpenShiftDockerRegistryConfigs) List(opts v1.ListOptions) (result *v1alpha1.OpenShiftDockerRegistryConfigList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(openshiftdockerregistryconfigsResource, openshiftdockerregistryconfigsKind, opts), &v1alpha1.OpenShiftDockerRegistryConfigList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.OpenShiftDockerRegistryConfigList{}
	for _, item := range obj.(*v1alpha1.OpenShiftDockerRegistryConfigList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested openShiftDockerRegistryConfigs.
func (c *FakeOpenShiftDockerRegistryConfigs) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(openshiftdockerregistryconfigsResource, opts))
}

// Create takes the representation of a openShiftDockerRegistryConfig and creates it.  Returns the server's representation of the openShiftDockerRegistryConfig, and an error, if there is any.
func (c *FakeOpenShiftDockerRegistryConfigs) Create(openShiftDockerRegistryConfig *v1alpha1.OpenShiftDockerRegistryConfig) (result *v1alpha1.OpenShiftDockerRegistryConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(openshiftdockerregistryconfigsResource, openShiftDockerRegistryConfig), &v1alpha1.OpenShiftDockerRegistryConfig{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.OpenShiftDockerRegistryConfig), err
}

// Update takes the representation of a openShiftDockerRegistryConfig and updates it. Returns the server's representation of the openShiftDockerRegistryConfig, and an error, if there is any.
func (c *FakeOpenShiftDockerRegistryConfigs) Update(openShiftDockerRegistryConfig *v1alpha1.OpenShiftDockerRegistryConfig) (result *v1alpha1.OpenShiftDockerRegistryConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(openshiftdockerregistryconfigsResource, openShiftDockerRegistryConfig), &v1alpha1.OpenShiftDockerRegistryConfig{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.OpenShiftDockerRegistryConfig), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeOpenShiftDockerRegistryConfigs) UpdateStatus(openShiftDockerRegistryConfig *v1alpha1.OpenShiftDockerRegistryConfig) (*v1alpha1.OpenShiftDockerRegistryConfig, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(openshiftdockerregistryconfigsResource, "status", openShiftDockerRegistryConfig), &v1alpha1.OpenShiftDockerRegistryConfig{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.OpenShiftDockerRegistryConfig), err
}

// Delete takes name of the openShiftDockerRegistryConfig and deletes it. Returns an error if one occurs.
func (c *FakeOpenShiftDockerRegistryConfigs) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(openshiftdockerregistryconfigsResource, name), &v1alpha1.OpenShiftDockerRegistryConfig{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeOpenShiftDockerRegistryConfigs) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(openshiftdockerregistryconfigsResource, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.OpenShiftDockerRegistryConfigList{})
	return err
}

// Patch applies the patch and returns the patched openShiftDockerRegistryConfig.
func (c *FakeOpenShiftDockerRegistryConfigs) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.OpenShiftDockerRegistryConfig, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(openshiftdockerregistryconfigsResource, name, data, subresources...), &v1alpha1.OpenShiftDockerRegistryConfig{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.OpenShiftDockerRegistryConfig), err
}
