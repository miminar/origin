package image

import (
	"testing"

	kapi "k8s.io/kubernetes/pkg/api"
	kquota "k8s.io/kubernetes/pkg/quota"

	"github.com/openshift/origin/pkg/client/testclient"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imagetest "github.com/openshift/origin/pkg/quota/image/testutil"
)

func TestImageStreamTagEvaluatorUsage(t *testing.T) {
	for _, tc := range []struct {
		name               string
		iss                []imageapi.ImageStream
		ist                imageapi.ImageStreamTag
		expectedSpecRefs   int64
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
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "is:dest",
				},
				Tag: &imageapi.TagReference{
					Name: "dest",
					From: &kapi.ObjectReference{
						Kind:      "ImageStreamImage",
						Namespace: "shared",
						Name:      "is@" + imagetest.MiscImageDigest,
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 1,
		},

		{
			name: "no image stream",
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "is:dest",
				},
				Tag: &imageapi.TagReference{
					Name: "dest",
					From: &kapi.ObjectReference{
						Kind:      "ImageStreamImage",
						Namespace: "shared",
						Name:      "is@" + imagetest.MiscImageDigest,
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 1,
		},

		{
			name: "no image stream using image stream tag",
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "is:dest",
				},
				Tag: &imageapi.TagReference{
					Name: "dest",
					From: &kapi.ObjectReference{
						Kind:      "ImageStreamTag",
						Namespace: "shared",
						Name:      "is:latest",
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 0,
		},

		{
			name: "no tag given",
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "is:dest",
				},
				Image: imageapi.Image{
					ObjectMeta: kapi.ObjectMeta{
						Name:        imagetest.MiscImageDigest,
						Annotations: map[string]string{imageapi.ManagedByOpenShiftAnnotation: "true"},
					},
					DockerImageReference: imagetest.MakeDockerImageReference("shared", "is", imagetest.MiscImageDigest),
					DockerImageManifest:  imagetest.MiscImageDigest,
				},
			},
			expectedSpecRefs:   0,
			expectedStatusRefs: 0,
		},

		{
			name: "missing from",
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "is:dest",
				},
				Tag: &imageapi.TagReference{
					Name: "dest",
				},
				Image: imageapi.Image{
					ObjectMeta: kapi.ObjectMeta{
						Name:        imagetest.MiscImageDigest,
						Annotations: map[string]string{imageapi.ManagedByOpenShiftAnnotation: "true"},
					},
					DockerImageReference: imagetest.MakeDockerImageReference("test", "dest", imagetest.MiscImageDigest),
					DockerImageManifest:  imagetest.MiscImage,
				},
			},
			expectedSpecRefs:   0,
			expectedStatusRefs: 0,
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
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "havingtag:latest",
				},
				Tag: &imageapi.TagReference{
					Name: "latest",
					From: &kapi.ObjectReference{
						Kind:      "ImageStreamImage",
						Namespace: "shared",
						Name:      "is@" + imagetest.ChildImageWith2LayersDigest,
					},
				},
			},
			expectedSpecRefs:   1,
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
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "destis:latest",
				},
				Tag: &imageapi.TagReference{
					Name: "latest",
					From: &kapi.ObjectReference{
						Kind:      "ImageStreamTag",
						Namespace: "other",
						Name:      "is2:latest",
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 0,
		},

		{
			name: "tag an image already present using image stream image",
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
			},
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "destis:latest",
				},
				Tag: &imageapi.TagReference{
					Name: "latest",
					From: &kapi.ObjectReference{
						Kind:      "ImageStreamImage",
						Namespace: "shared",
						Name:      "is@" + imagetest.BaseImageWith1LayerDigest,
					},
				},
			},
			// image stream image is a unique reference
			expectedSpecRefs:   1,
			expectedStatusRefs: 0,
		},

		{
			name: "tag an image already present using image stream tag",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "destis",
					},
					Spec: imageapi.ImageStreamSpec{
						Tags: map[string]imageapi.TagReference{
							"new": {
								Name: "new",
								From: &kapi.ObjectReference{
									Kind:      "ImageStreamTag",
									Namespace: "shared",
									Name:      "is:latest",
								},
							},
						},
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
			},
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "another:latest",
				},
				Tag: &imageapi.TagReference{
					Name: "latest",
					// shared is has name of baseImageWith1Layer at the first place in event list
					From: &kapi.ObjectReference{
						Kind:      "ImageStreamTag",
						Namespace: "shared",
						Name:      "is:latest",
					},
				},
			},
			expectedSpecRefs:   0,
			expectedStatusRefs: 0,
		},

		{
			name: "tag a dockerimage already present using istag",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "is",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "is", imagetest.BaseImageWith1LayerDigest),
										Image:                imagetest.BaseImageWith1LayerDigest,
									},
								},
							},
						},
					},
				},
			},
			ist: imageapi.ImageStreamTag{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "another:latest",
				},
				Tag: &imageapi.TagReference{
					Name: "latest",
					// shared is has name of baseImageWith1Layer at the first place in event list
					From: &kapi.ObjectReference{
						Kind: "DockerImage",
						Name: imagetest.MakeDockerImageReference("test", "is", imagetest.BaseImageWith1LayerDigest),
					},
				},
			},
			// the first time the reference is tagged to a spec
			expectedSpecRefs: 1,
			// already present in the status
			expectedStatusRefs: 0,
		},
	} {

		fakeClient := &testclient.Fake{}
		fakeClient.AddReactor("list", "imagestreams", imagetest.GetFakeImageStreamListHandler(t, tc.iss...))

		evaluator := NewImageStreamTagEvaluator(fakeClient)

		usage := evaluator.Usage(&tc.ist)

		expectedUsage := imagetest.ExpectedResourceListFor(tc.expectedSpecRefs, tc.expectedStatusRefs)
		expectedResources := kquota.ResourceNames(expectedUsage)
		if len(usage) != len(expectedUsage) {
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
