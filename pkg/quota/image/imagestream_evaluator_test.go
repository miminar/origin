package image

import (
	"fmt"
	"testing"

	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	kquota "k8s.io/kubernetes/pkg/quota"

	"github.com/openshift/origin/pkg/client/testclient"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imagetest "github.com/openshift/origin/pkg/quota/image/testutil"
)

func TestImageStreamEvaluatorUsage(t *testing.T) {
	for _, tc := range []struct {
		name               string
		is                 imageapi.ImageStream
		expectedSpecRefs   int64
		expectedStatusRefs int64
	}{
		{
			name: "empty image stream",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "empty",
				},
				Status: imageapi.ImageStreamStatus{},
			},
			expectedSpecRefs:   0,
			expectedStatusRefs: 0,
		},

		{
			name: "is with one tag",
			is: imageapi.ImageStream{
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
			expectedSpecRefs:   0,
			expectedStatusRefs: 1,
		},

		{
			name: "is with spec filled",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "is",
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"new": {
							Name: "new",
							From: &kapi.ObjectReference{
								Kind:      "ImageStreamImage",
								Namespace: "shared",
								Name:      fmt.Sprintf("is@%s", imagetest.MiscImageDigest),
							},
						},
					},
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
			expectedSpecRefs:   1,
			expectedStatusRefs: 1,
		},

		{
			name: "two images under one tag",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "sharedlayer",
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.BaseImageWith1LayerDigest),
									Image:                imagetest.BaseImageWith1LayerDigest,
								},
								{
									DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.BaseImageWith2LayersDigest),
									Image:                imagetest.BaseImageWith2LayersDigest,
								},
							},
						},
					},
				},
			},
			expectedSpecRefs:   0,
			expectedStatusRefs: 2,
		},

		{
			name: "two different tags",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "sharedlayer",
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"foo": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.BaseImageWith2LayersDigest),
									Image:                imagetest.BaseImageWith2LayersDigest,
								},
							},
						},
						"bar": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.ChildImageWith3LayersDigest),
									Image:                imagetest.ChildImageWith3LayersDigest,
								},
							},
						},
					},
				},
			},
			expectedSpecRefs:   0,
			expectedStatusRefs: 2,
		},

		{
			name: "the same image under different tags",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "noshared",
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: imagetest.MakeDockerImageReference("test", "noshared", imagetest.ChildImageWith2LayersDigest),
									Image:                imagetest.ChildImageWith2LayersDigest,
								},
							},
						},
						"foo": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: imagetest.MakeDockerImageReference("test", "noshared", imagetest.ChildImageWith2LayersDigest),
									Image:                imagetest.ChildImageWith2LayersDigest,
								},
							},
						},
					},
				},
			},
			expectedSpecRefs:   0,
			expectedStatusRefs: 1,
		},

		{
			name: "imagestreamimage reference pointing to image present in status",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "noshared",
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"new": {
							Name: "new",
							From: &kapi.ObjectReference{
								Kind:      "ImageStreamImage",
								Namespace: "shared",
								Name:      fmt.Sprintf("is@%s", imagetest.ChildImageWith3LayersDigest),
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: imagetest.MakeDockerImageReference("test", "noshared", imagetest.ChildImageWith3LayersDigest),
									Image:                imagetest.ChildImageWith3LayersDigest,
								},
							},
						},
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 1,
		},

		{
			name: "two non-canonical references",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "is",
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"new": {
							Name: "new",
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "repo:latest",
							},
						},
						"same": {
							Name: "same",
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: "index.docker.io/repo",
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"new": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: "docker.io/library/repo:latest",
									Image:                imagetest.ChildImageWith3LayersDigest,
								},
							},
						},
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 1,
		},

		{
			name: "the same image in both spec and status",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "noshared",
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"new": {
							Name: "new",
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: imagetest.MakeDockerImageReference("test", "noshared", imagetest.ChildImageWith2LayersDigest),
							},
						},
					},
				},
				Status: imageapi.ImageStreamStatus{
					Tags: map[string]imageapi.TagEventList{
						"latest": {
							Items: []imageapi.TagEvent{
								{
									DockerImageReference: imagetest.MakeDockerImageReference("test", "noshared", imagetest.ChildImageWith2LayersDigest),
									Image:                imagetest.ChildImageWith2LayersDigest,
								},
							},
						},
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 1,
		},

		{
			name: "imagestreamtag and dockerimage references",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "noshared",
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"ist": {
							Name: "ist",
							From: &kapi.ObjectReference{
								Kind:      "ImageStreamTag",
								Namespace: "shared",
								Name:      "is:latest",
							},
						},
						"dockerimage": {
							Name: "dockerimage",
							From: &kapi.ObjectReference{
								Kind:      "DockerImage",
								Namespace: "shared",
								Name:      fmt.Sprintf("is:latest"),
							},
						},
					},
				},
			},
			expectedSpecRefs:   2,
			expectedStatusRefs: 0,
		},

		{
			name: "dockerimage reference tagged in status",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "noshared",
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"dockerimage": {
							Name: "dockerimage",
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: imagetest.MakeDockerImageReference("test", "is", imagetest.BaseImageWith1LayerDigest),
							},
						},
					},
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
			expectedSpecRefs:   1,
			expectedStatusRefs: 1,
		},

		{
			name: "wrong spec image references",
			is: imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "noshared",
				},
				Spec: imageapi.ImageStreamSpec{
					Tags: map[string]imageapi.TagReference{
						"badkind": {
							Name: "badkind",
							From: &kapi.ObjectReference{
								Kind: "unknown",
								Name: imagetest.MakeDockerImageReference("test", "is", imagetest.BaseImageWith1LayerDigest),
							},
						},
						"badistag": {
							Name: "badistag",
							From: &kapi.ObjectReference{
								Kind:      "ImageStreamTag",
								Namespace: "shared",
								Name:      "is",
							},
						},
						"badisimage": {
							Name: "badistag",
							From: &kapi.ObjectReference{
								Kind:      "ImageStreamImage",
								Namespace: "shared",
								Name:      "is:tag",
							},
						},
						"good": {
							Name: "good",
							From: &kapi.ObjectReference{
								Kind: "DockerImage",
								Name: imagetest.MakeDockerImageReference("test", "is", imagetest.BaseImageWith1LayerDigest),
							},
						},
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 0,
		},
	} {
		fakeClient := &testclient.Fake{}
		fakeClient.AddReactor("get", "imagestreams", imagetest.GetFakeImageStreamGetHandler(t, tc.is))

		evaluator := NewImageStreamEvaluator(fakeClient)

		is, err := evaluator.Get(tc.is.Namespace, tc.is.Name)
		if err != nil {
			t.Errorf("[%s]: could not get image stream %q: %v", tc.name, tc.is.Name, err)
			continue
		}
		usage := evaluator.Usage(is)

		expectedUsage := kapi.ResourceList{
			imageapi.ResourceImageStreamTags:   *resource.NewQuantity(tc.expectedSpecRefs, resource.DecimalSI),
			imageapi.ResourceImageStreamImages: *resource.NewQuantity(tc.expectedStatusRefs, resource.DecimalSI),
		}

		expectedResources := kquota.ResourceNames(expectedUsage)
		if len(usage) != len(expectedResources) {
			t.Errorf("[%s]: got unexpected number of computed resources: %d != %d", tc.name, len(usage), len(expectedResources))
		}
		masked := kquota.Mask(usage, expectedResources)

		if len(masked) != len(expectedResources) {
			for k := range usage {
				if _, exists := masked[k]; !exists {
					t.Errorf("[%s]: got unexpected resource %q from Usage() method", tc.name, k)
				}
			}

			for _, k := range expectedResources {
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

func TestImageStreamEvaluatorUsageStats(t *testing.T) {
	for _, tc := range []struct {
		name               string
		iss                []imageapi.ImageStream
		namespace          string
		expectedSpecRefs   int64
		expectedStatusRefs int64
	}{
		{
			name:               "no image stream",
			iss:                []imageapi.ImageStream{},
			namespace:          "test",
			expectedSpecRefs:   0,
			expectedStatusRefs: 0,
		},

		{
			name: "one is with one tag",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "onetag",
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
			namespace:          "test",
			expectedSpecRefs:   0,
			expectedStatusRefs: 1,
		},

		{
			name: "is with two references under one tag",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "sharedlayer",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.BaseImageWith1LayerDigest),
										Image:                imagetest.BaseImageWith1LayerDigest,
									},
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.BaseImageWith2LayersDigest),
										Image:                imagetest.BaseImageWith2LayersDigest,
									},
								},
							},
						},
					},
				},
			},
			namespace:          "test",
			expectedSpecRefs:   0,
			expectedStatusRefs: 2,
		},

		{
			name: "two images in two image streams",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "is1",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "is1", imagetest.BaseImageWith1LayerDigest),
										Image:                imagetest.BaseImageWith1LayerDigest,
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
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
			namespace:          "test",
			expectedSpecRefs:   0,
			expectedStatusRefs: 2,
		},

		{
			name: "two image streams in different namespaces",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "is1",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "is1", imagetest.ChildImageWith2LayersDigest),
										Image:                imagetest.ChildImageWith2LayersDigest,
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
										DockerImageReference: imagetest.MakeDockerImageReference("test", "is2", imagetest.MiscImageDigest),
										Image:                imagetest.MiscImageDigest,
									},
								},
							},
						},
					},
				},
			},
			namespace:          "test",
			expectedSpecRefs:   0,
			expectedStatusRefs: 1,
		},

		{
			name: "same image in two different image streams",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "is1",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "is1", imagetest.ChildImageWith2LayersDigest),
										Image:                imagetest.ChildImageWith2LayersDigest,
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "is2",
					},
					Status: imageapi.ImageStreamStatus{
						Tags: map[string]imageapi.TagEventList{
							"latest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "is1", imagetest.MiscImageDigest),
										Image:                imagetest.MiscImageDigest,
									},
								},
							},
							"foo": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: imagetest.MakeDockerImageReference("test", "is1", imagetest.ChildImageWith2LayersDigest),
										Image:                imagetest.ChildImageWith2LayersDigest,
									},
								},
							},
						},
					},
				},
			},
			namespace:          "test",
			expectedSpecRefs:   0,
			expectedStatusRefs: 2,
		},
	} {
		fakeClient := &testclient.Fake{}
		fakeClient.AddReactor("list", "imagestreams", imagetest.GetFakeImageStreamListHandler(t, tc.iss...))

		evaluator := NewImageStreamEvaluator(fakeClient)

		stats, err := evaluator.UsageStats(kquota.UsageStatsOptions{Namespace: tc.namespace})
		if err != nil {
			t.Errorf("[%s]: could not get usage stats for namespace %q: %v", tc.name, tc.namespace, err)
			continue
		}

		expectedUsage := imagetest.ExpectedResourceListFor(tc.expectedSpecRefs, tc.expectedStatusRefs)
		expectedResources := kquota.ResourceNames(expectedUsage)
		if len(stats.Used) != len(expectedResources) {
			t.Errorf("[%s]: got unexpected number of computed resources: %d != %d", tc.name, len(stats.Used), len(expectedResources))
		}
		masked := kquota.Mask(stats.Used, expectedResources)

		if len(masked) != len(expectedResources) {
			for k := range stats.Used {
				if _, exists := masked[k]; !exists {
					t.Errorf("[%s]: got unexpected resource %q from Usage() method", tc.name, k)
				}
			}

			for _, k := range expectedResources {
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

func TestImageStreamAdmissionEvaluatorUsage(t *testing.T) {
	for _, tc := range []struct {
		name               string
		spec               *imageapi.ImageStreamSpec
		status             *imageapi.ImageStreamStatus
		expectedSpecRefs   int64
		expectedStatusRefs int64
	}{
		{
			name:   "empty image stream",
			status: &imageapi.ImageStreamStatus{},
		},

		{
			name: "one image",
			status: &imageapi.ImageStreamStatus{
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
			expectedStatusRefs: 1,
		},

		{
			name: "no change",
			spec: &imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"new": {
						Name: "new",
						From: &kapi.ObjectReference{
							Kind:      "ImageStreamImage",
							Namespace: "shared",
							Name:      fmt.Sprintf("is@%s", imagetest.MiscImageDigest),
						},
					},
				},
			},
			status: &imageapi.ImageStreamStatus{
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
			expectedSpecRefs: 1,
			// misc image is already present in the common IS
			expectedStatusRefs: 1,
		},

		{
			name: "three tags",
			status: &imageapi.ImageStreamStatus{
				Tags: map[string]imageapi.TagEventList{
					"latest": {
						Items: []imageapi.TagEvent{
							{
								DockerImageReference: imagetest.MakeDockerImageReference("test", "is", imagetest.BaseImageWith1LayerDigest),
								Image:                imagetest.BaseImageWith1LayerDigest,
							},
						},
					},
					"foo": {
						Items: []imageapi.TagEvent{
							{
								DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.ChildImageWith2LayersDigest),
								Image:                imagetest.ChildImageWith2LayersDigest,
							},
						},
					},
					"bar": {
						Items: []imageapi.TagEvent{
							{
								DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.BaseImageWith2LayersDigest),
								Image:                imagetest.BaseImageWith2LayersDigest,
							},
						},
					},
				},
			},
			expectedStatusRefs: 3,
		},

		{
			name: "two items under one tag",
			status: &imageapi.ImageStreamStatus{
				Tags: map[string]imageapi.TagEventList{
					"foo": {
						Items: []imageapi.TagEvent{
							{
								DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.ChildImageWith3LayersDigest),
								Image:                imagetest.ChildImageWith3LayersDigest,
							},
							{
								DockerImageReference: imagetest.MakeDockerImageReference("test", "sharedlayer", imagetest.MiscImageDigest),
								Image:                imagetest.MiscImageDigest,
							},
						},
					},
				},
			},
			// misc image is already present in common is
			expectedStatusRefs: 1,
		},

		{
			name: "image in spec and status",
			spec: &imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"new": {
						Name: "new",
						From: &kapi.ObjectReference{
							Kind:      "ImageStreamImage",
							Namespace: "shared",
							Name:      fmt.Sprintf("is@%s", imagetest.BaseImageWith2LayersDigest),
						},
					},
				},
			},
			status: &imageapi.ImageStreamStatus{
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
			expectedSpecRefs:   1,
			expectedStatusRefs: 2,
		},

		{
			name: "same image in spec and status",
			spec: &imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"latest": {
						Name: "new",
						From: &kapi.ObjectReference{
							Kind:      "ImageStreamImage",
							Namespace: "shared",
							Name:      fmt.Sprintf("is@%s", imagetest.ChildImageWith2LayersDigest),
						},
					},
				},
			},
			status: &imageapi.ImageStreamStatus{
				Tags: map[string]imageapi.TagEventList{
					"latest": {
						Items: []imageapi.TagEvent{
							{
								DockerImageReference: imagetest.MakeDockerImageReference("test", "is", imagetest.BaseImageWith2LayersDigest),
								Image:                imagetest.ChildImageWith2LayersDigest,
							},
						},
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 1,
		},

		{
			name: "refer to image in another namespace already present",
			spec: &imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"misc": {
						Name: "misc",
						From: &kapi.ObjectReference{
							Kind:      "ImageStreamImage",
							Namespace: "shared",
							Name:      fmt.Sprintf("is@%s", imagetest.MiscImageDigest),
						},
					},
				},
			},
			expectedSpecRefs: 1,
			// misc image already present in the common IS
			expectedStatusRefs: 0,
		},

		{
			name: "refer to image in the same namespace using dockerimage",
			spec: &imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"misc": {
						Name: "misc",
						From: &kapi.ObjectReference{
							Kind: "DockerImage",
							Name: imagetest.MakeDockerImageReference("test", "common", imagetest.MiscImageDigest),
						},
					},
				},
			},
			// it's the first time the reference occurs in spec
			expectedSpecRefs:   1,
			expectedStatusRefs: 0,
		},

		{
			name: "dockerimage reference duplicated",
			spec: &imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"latest": {
						Name: "latest",
						From: &kapi.ObjectReference{
							Kind: "DockerImage",
							Name: imagetest.MakeDockerImageReference("test", "other", imagetest.ChildImageWith2LayersDigest),
						},
					},
				},
			},
			expectedSpecRefs:   0,
			expectedStatusRefs: 0,
		},

		{
			name: "refer to imagestreamimage in the same namespace",
			spec: &imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"commonisi": {
						Name: "commonisi",
						From: &kapi.ObjectReference{
							Kind: "ImageStreamImage",
							Name: fmt.Sprintf("common@%s", imagetest.MiscImageDigest),
						},
					},
				},
			},
			expectedSpecRefs:   1,
			expectedStatusRefs: 0,
		},

		{
			name: "refer to imagestreamtag in the same namespace",
			spec: &imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"commonist": {
						Name: "commonist",
						From: &kapi.ObjectReference{
							Kind: "ImageStreamTag",
							Name: "common:misc",
						},
					},
				},
			},
			// we don't attempt to resolve image stream tags
			expectedSpecRefs:   1,
			expectedStatusRefs: 0,
		},
	} {

		commonIS := imageapi.ImageStream{
			ObjectMeta: kapi.ObjectMeta{
				Namespace: "test",
				Name:      "common",
			},
			Spec: imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"bar": {
						Name: "bar",
						From: &kapi.ObjectReference{
							Kind: "DockerImage",
							Name: imagetest.MakeDockerImageReference("test", "other", imagetest.ChildImageWith2LayersDigest),
						},
					},
				},
			},
			Status: imageapi.ImageStreamStatus{
				Tags: map[string]imageapi.TagEventList{
					"misc": {
						Items: []imageapi.TagEvent{
							{
								DockerImageReference: imagetest.MakeDockerImageReference("test", "common", imagetest.MiscImageDigest),
								Image:                imagetest.MiscImageDigest,
							},
						},
					},
				},
			},
		}

		oldIS := imageapi.ImageStream{
			ObjectMeta: kapi.ObjectMeta{
				Namespace: "test",
				Name:      "is",
			},
			Spec: imageapi.ImageStreamSpec{
				Tags: map[string]imageapi.TagReference{
					"new": {
						Name: "new",
						From: &kapi.ObjectReference{
							Kind:      "ImageStreamImage",
							Namespace: "shared",
							Name:      fmt.Sprintf("is@%s", imagetest.MiscImageDigest),
						},
					},
				},
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
		}
		iss := []imageapi.ImageStream{oldIS, commonIS}

		var newIS *imageapi.ImageStream
		if tc.status != nil || tc.spec != nil {
			newIS = &imageapi.ImageStream{
				ObjectMeta: kapi.ObjectMeta{
					Namespace: "test",
					Name:      "is",
				},
			}
			if tc.spec != nil {
				newIS.Spec = *tc.spec
			}
			if tc.status != nil {
				newIS.Status = *tc.status
			}
		}

		fakeClient := &testclient.Fake{}
		fakeClient.AddReactor("get", "imagestreams", imagetest.GetFakeImageStreamGetHandler(t, iss...))
		fakeClient.AddReactor("list", "imagestreams", imagetest.GetFakeImageStreamListHandler(t, iss...))

		evaluator := NewImageStreamAdmissionEvaluator(fakeClient)

		usage := evaluator.Usage(newIS)

		expectedUsage := imagetest.ExpectedResourceListFor(tc.expectedSpecRefs, tc.expectedStatusRefs)
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
