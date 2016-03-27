package image

import (
	"testing"

	kapi "k8s.io/kubernetes/pkg/api"
	kquota "k8s.io/kubernetes/pkg/quota"

	"github.com/openshift/origin/pkg/client/testclient"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imagetest "github.com/openshift/origin/pkg/quota/image/testutil"
)

func TestImageStreamMappingEvaluatorUsage(t *testing.T) {
	for _, tc := range []struct {
		name               string
		iss                []imageapi.ImageStream
		imageName          string
		imageManifest      string
		imageAnnotations   map[string]string
		destISNamespace    string
		destISName         string
		expectedStatusRefs int64
	}{
		{
			name: "empty image stream",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "is",
					},
					Status: imageapi.ImageStreamStatus{},
				},
			},
			imageName:          imagetest.MiscImageDigest,
			imageManifest:      imagetest.MiscImage,
			imageAnnotations:   map[string]string{imageapi.ManagedByOpenShiftAnnotation: "true"},
			destISNamespace:    "test",
			destISName:         "is",
			expectedStatusRefs: 1,
		},

		{
			name:             "no image stream",
			imageName:        imagetest.MiscImageDigest,
			imageManifest:    imagetest.MiscImage,
			imageAnnotations: map[string]string{imageapi.ManagedByOpenShiftAnnotation: "true"},
			destISNamespace:  "test",
			destISName:       "is",
			// there must be no increment if the image stream doesn't already exist
			expectedStatusRefs: 0,
		},

		{
			name: "missing image annotation",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "is",
					},
					Status: imageapi.ImageStreamStatus{},
				},
			},
			imageName:       imagetest.MiscImageDigest,
			imageManifest:   imagetest.MiscImage,
			destISNamespace: "test",
			destISName:      "is",
			// we don't differentiate between internal and external references
			expectedStatusRefs: 1,
		},

		{
			name: "update existing tag",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "havingtag",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "havingtag", imagetest.BaseImageWith1LayerDigest),
										Image:                imagetest.BaseImageWith1LayerDigest,
									},
								},
							},
						},
					},
				},
			},
			imageName:          imagetest.ChildImageWith2LayersDigest,
			imageManifest:      imagetest.ChildImageWith2Layers,
			imageAnnotations:   map[string]string{imageapi.ManagedByOpenShiftAnnotation: "true"},
			destISNamespace:    "test",
			destISName:         "havingtag",
			expectedStatusRefs: 1,
		},

		{
			name: "add a new tag with 2 image streams",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "destis",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "destis", imagetest.BaseImageWith1LayerDigest),
										Image:                imagetest.BaseImageWith1LayerDigest,
									},
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "is2", imagetest.MiscImageDigest),
										Image:                imagetest.MiscImageDigest,
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "other",
						Name:      "is2",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "is2", imagetest.BaseImageWith2LayersDigest),
										Image:                imagetest.BaseImageWith2LayersDigest,
									},
								},
							},
						},
					},
				},
			},
			imageName:          imagetest.ChildImageWith3LayersDigest,
			imageManifest:      imagetest.ChildImageWith3Layers,
			imageAnnotations:   map[string]string{imageapi.ManagedByOpenShiftAnnotation: "true"},
			destISNamespace:    "test",
			destISName:         "destis",
			expectedStatusRefs: 1,
		},

		{
			name: "add a new tag to an image stream with image present in the other",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "other",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "other", imagetest.BaseImageWith2LayersDigest),
										Image:                imagetest.BaseImageWith2LayersDigest,
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "destis",
					},
				},
			},
			imageName:          imagetest.BaseImageWith2LayersDigest,
			imageManifest:      imagetest.BaseImageWith2Layers,
			imageAnnotations:   map[string]string{imageapi.ManagedByOpenShiftAnnotation: "true"},
			destISNamespace:    "test",
			destISName:         "destis",
			expectedStatusRefs: 0,
		},
	} {

		fakeClient := &testclient.Fake{}
		fakeClient.AddReactor("list", "imagestreams", imagetest.GetFakeImageStreamListHandler(t, tc.iss...))
		fakeClient.AddReactor("get", "imagestreams", imagetest.GetFakeImageStreamGetHandler(t, tc.iss...))

		evaluator := NewImageStreamMappingEvaluator(fakeClient)

		ism := &imageapi.ImageStreamMapping{
			ObjectMeta: kapi.ObjectMeta{
				Namespace: tc.destISNamespace,
				Name:      tc.destISName,
			},
			Image: imageapi.Image{
				ObjectMeta: kapi.ObjectMeta{
					Name:        tc.imageName,
					Annotations: tc.imageAnnotations,
				},
				DockerImageReference: imagetest.MakeDockerImageReference(tc.destISNamespace, tc.destISName, tc.imageName),
				DockerImageManifest:  tc.imageManifest,
			},
		}

		usage := evaluator.Usage(ism)

		expectedUsage := imagetest.ExpectedResourceListFor(0, tc.expectedStatusRefs)
		expectedResources := kquota.ResourceNames(expectedUsage)
		if len(usage) != len(expectedResources) {
			t.Errorf("[%s]: got unexpected number of computed resources: %d != %d", tc.name, len(usage), len(expectedResources))
		}

		masked := kquota.Mask(usage, expectedResources)
		if len(masked) != len(expectedUsage) {
			for k := range usage {
				if _, exists := masked[k]; !exists {
					t.Errorf("[%s]: got unexpected resource %q from Usage() method", tc.name, k)
				}
			}

			for k := range expectedUsage {
				if _, exists := masked[k]; !exists {
					t.Errorf("[%s]: expected resource %q not computed", tc.name, k)
				}
			}
		}

		for rname, expectedValue := range expectedUsage {
			if v, exists := masked[rname]; exists {
				if v.Cmp(expectedValue) != 0 {
					t.Errorf("[%s]: got unexpected usage for %q: %s != %s", tc.name, rname, v.String(), expectedValue.String())
				}
			}
		}
	}
}
