package image

import (
	"testing"

	kapi "k8s.io/kubernetes/pkg/api"

	"github.com/openshift/origin/pkg/client/testclient"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imagetest "github.com/openshift/origin/pkg/quota/image/testutil"
)

func TestGetImageReferenceForObjectReference(t *testing.T) {
	for _, tc := range []struct {
		name           string
		namespace      string
		objRef         kapi.ObjectReference
		expectedString string
		expectedError  bool
	}{
		{
			name: "isimage without namespace",
			objRef: kapi.ObjectReference{
				Kind: "ImageStreamImage",
				Name: imageapi.MakeImageStreamImageName("is", imagetest.BaseImageWith1LayerDigest),
			},
			expectedString: "is@" + imagetest.BaseImageWith1LayerDigest,
		},

		{
			name:      "isimage with a fallback namespace",
			namespace: "fallback",
			objRef: kapi.ObjectReference{
				Kind: "ImageStreamImage",
				Name: imageapi.MakeImageStreamImageName("is", imagetest.BaseImageWith1LayerDigest),
			},
			expectedString: "fallback/is@" + imagetest.BaseImageWith1LayerDigest,
		},

		{
			name:      "isimage with namespace set",
			namespace: "fallback",
			objRef: kapi.ObjectReference{
				Kind:      "ImageStreamImage",
				Namespace: "ns",
				Name:      imageapi.MakeImageStreamImageName("is", imagetest.BaseImageWith1LayerDigest),
			},
			expectedString: "ns/is@" + imagetest.BaseImageWith1LayerDigest,
		},

		{
			name: "isimage missing id",
			objRef: kapi.ObjectReference{
				Kind: "ImageStreamImage",
				Name: imagetest.InternalRegistryURL + "/is",
			},
			expectedError: true,
		},

		{
			name: "isimage with a tag",
			objRef: kapi.ObjectReference{
				Kind: "ImageStreamImage",
				Name: imagetest.InternalRegistryURL + "/is:latest",
			},
			expectedError: true,
		},

		{
			name: "istag without namespace",
			objRef: kapi.ObjectReference{
				Kind: "ImageStreamTag",
				Name: "is:latest",
			},
			expectedString: "is:latest",
		},

		{
			name:      "istag with fallback namespace",
			namespace: "fallback",
			objRef: kapi.ObjectReference{
				Kind: "ImageStreamTag",
				Name: "is:latest",
			},
			expectedString: "fallback/is:latest",
		},

		{
			name:      "istag with namespace set",
			namespace: "fallback",
			objRef: kapi.ObjectReference{
				Kind:      "ImageStreamTag",
				Namespace: "ns",
				Name:      "is:latest",
			},
			expectedString: "ns/is:latest",
		},

		{
			name: "istag with missing tag",
			objRef: kapi.ObjectReference{
				Kind: "ImageStreamTag",
				Name: "is",
			},
			expectedError: true,
		},

		{
			name: "istag with image ID",
			objRef: kapi.ObjectReference{
				Kind: "ImageStreamTag",
				Name: "is@" + imagetest.BaseImageWith1LayerDigest,
			},
			expectedError: true,
		},

		{
			name: "dockerimage without registry url",
			objRef: kapi.ObjectReference{
				Kind:      "DockerImage",
				Namespace: "ns",
				Name:      "repo@" + imagetest.BaseImageWith1LayerDigest,
			},
			expectedString: "docker.io/repo@" + imagetest.BaseImageWith1LayerDigest,
		},

		{
			name: "dockerimage with a default tag",
			objRef: kapi.ObjectReference{
				Kind:      "DockerImage",
				Namespace: "ns",
				Name:      "library/repo:latest",
			},
			expectedString: "docker.io/repo",
		},

		{
			name: "dockerimage with a non-default tag",
			objRef: kapi.ObjectReference{
				Kind:      "DockerImage",
				Namespace: "ns",
				Name:      "repo:tag",
			},
			expectedString: "docker.io/repo:tag",
		},

		{
			name: "dockerimage referencing docker image",
			objRef: kapi.ObjectReference{
				Kind: "DockerImage",
				Name: "index.docker.io/repo@" + imagetest.BaseImageWith1LayerDigest,
			},
			expectedString: "docker.io/repo@" + imagetest.BaseImageWith1LayerDigest,
		},

		{
			name: "dockerimage without tag or id",
			objRef: kapi.ObjectReference{
				Kind: "DockerImage",
				Name: "index.docker.io/user/repo",
			},
			expectedString: "docker.io/user/repo",
		},

		{
			name: "dockerimage with internal registry",
			objRef: kapi.ObjectReference{
				Kind: "DockerImage",
				Name: imagetest.MakeDockerImageReference("test", "is", imagetest.BaseImageWith1LayerDigest),
			},
			expectedString: imagetest.InternalRegistryURL + "/test/is@" + imagetest.BaseImageWith1LayerDigest,
		},

		{
			name: "bad king",
			objRef: kapi.ObjectReference{
				Kind: "dockerImage",
				Name: imagetest.MakeDockerImageReference("test", "is", imagetest.BaseImageWith1LayerDigest),
			},
			expectedError: true,
		},
	} {

		fakeClient := &testclient.Fake{}
		c := NewGenericImageStreamUsageComputer(fakeClient)
		res, err := c.GetImageReferenceForObjectReference(tc.namespace, &tc.objRef)
		if tc.expectedError && err == nil {
			t.Errorf("[%s] got unexpected non-error", tc.name)
		}
		if !tc.expectedError {
			if err != nil {
				t.Errorf("[%s] got unexpected error: %v", tc.name, err)
			}
			if res != tc.expectedString {
				t.Errorf("[%s] got unexpected results (%q != %q)", tc.name, res, tc.expectedString)
			}
		}
	}
}
