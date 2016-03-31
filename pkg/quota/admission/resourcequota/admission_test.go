package resourcequota

import (
	"testing"

	"k8s.io/kubernetes/pkg/admission"
	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	kfake "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset/fake"

	"github.com/openshift/origin/pkg/client/testclient"
	imageapi "github.com/openshift/origin/pkg/image/api"
	"github.com/openshift/origin/pkg/quota"
	quotatestutil "github.com/openshift/origin/pkg/quota/image/testutil"
	quotautil "github.com/openshift/origin/pkg/quota/util"
)

// TestOriginQuotaAdmissionIsErrorQuotaExceeded verifies that if a resource exceeds allowed usage, the
// admission will return error we can recognize.
func TestOriginQuotaAdmissionIsErrorQuotaExceeded(t *testing.T) {
	resourceQuota := &kapi.ResourceQuota{
		ObjectMeta: kapi.ObjectMeta{Name: "quota", Namespace: "test", ResourceVersion: "124"},
		Status: kapi.ResourceQuotaStatus{
			Hard: kapi.ResourceList{
				imageapi.ResourceImageStreamImages: resource.MustParse("0"),
			},
			Used: kapi.ResourceList{
				imageapi.ResourceImageStreamImages: resource.MustParse("0"),
			},
		},
	}
	kubeClient := kfake.NewSimpleClientset(resourceQuota)
	osClient := testclient.NewSimpleFake(&imageapi.ImageStream{})
	plugin := NewOriginResourceQuota(kubeClient).(*originQuotaAdmission)
	plugin.SetOriginQuotaRegistry(quota.NewOriginQuotaRegistryForAdmission(osClient, 0))
	if err := plugin.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newIS := &imageapi.ImageStream{
		ObjectMeta: kapi.ObjectMeta{
			Namespace: "test",
			Name:      "is",
		},
		Status: imageapi.ImageStreamStatus{
			Tags: map[string]imageapi.TagEventList{
				"latest": {
					Items: []imageapi.TagEvent{
						{
							DockerImageReference: "127.0.0.1:5000/some/repository:tag",
							Image:                quotatestutil.MiscImageDigest,
						},
					},
				},
			},
		},
	}

	err := plugin.Admit(admission.NewAttributesRecord(newIS, kapi.Kind("ImageStream"), newIS.Namespace, newIS.Name, kapi.Resource("imageStreams"), "", admission.Create, nil))
	if err == nil {
		t.Fatalf("Expected an error exceeding quota")
	}
	if !quotautil.IsErrorQuotaExceeded(err) {
		t.Fatalf("Expected error %q to be matched by IsErrorQuotaExceeded()", err.Error())
	}
}
