package image

import (
	"testing"

	kapi "k8s.io/kubernetes/pkg/api"
	kquota "k8s.io/kubernetes/pkg/quota"

	"github.com/openshift/origin/pkg/client/testclient"
	imageapi "github.com/openshift/origin/pkg/image/api"
	imagetest "github.com/openshift/origin/pkg/quota/image/testutil"
)

const maxTestImportTagsPerRepository = 5

func TestImageStreamImportEvaluatorUsage(t *testing.T) {
	for _, tc := range []struct {
		name               string
		iss                []imageapi.ImageStream
		isiSpec            imageapi.ImageStreamImportSpec
		expectedSpecRefs   int64
		expectedStatusRefs int64
	}{
		{
			name: "nothing to import",
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: true,
			},
		},

		{
			name: "dry run",
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: false,
				Repository: &imageapi.RepositoryImportSpec{
					From: kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "docker.io/library/fedora",
					},
				},
			},
		},

		{
			name: "wrong from kind",
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: true,
				Repository: &imageapi.RepositoryImportSpec{
					From: kapi.ObjectReference{
						Kind:      "ImageStreamImage",
						Namespace: "test",
						Name:      imageapi.MakeImageStreamImageName("someis", imagetest.BaseImageWith1LayerDigest),
					},
				},
			},
		},

		{
			name: "import from repository to empty project",
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: true,
				Repository: &imageapi.RepositoryImportSpec{
					From: kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "docker.io/fedora",
					},
				},
			},
			expectedSpecRefs:   maxTestImportTagsPerRepository,
			expectedStatusRefs: maxTestImportTagsPerRepository,
		},

		{
			name: "import from repository with a tag imported",
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
							"foo": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: "docker.io/library/fedora:rawhide",
										Image:                imagetest.BaseImageWith2LayersDigest,
									},
								},
							},
							// doesn't count
							"digest": {
								Items: []imageapi.TagEvent{
									{
										DockerImageReference: "docker.io/library/fedora@" + imagetest.ChildImageWith2LayersDigest,
										Image:                imagetest.ChildImageWith2LayersDigest,
									},
								},
							},
						},
					},
				},
			},
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: true,
				Repository: &imageapi.RepositoryImportSpec{
					From: kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "docker.io/fedora",
					},
				},
			},
			expectedSpecRefs:   maxTestImportTagsPerRepository,
			expectedStatusRefs: maxTestImportTagsPerRepository,
		},

		{
			name: "import from repository with tags in spec",
			iss: []imageapi.ImageStream{
				{
					ObjectMeta: kapi.ObjectMeta{
						Namespace: "test",
						Name:      "spec",
					},
					Spec: imageapi.ImageStreamSpec{
						Tags: map[string]imageapi.TagReference{
							"latest": {
								Name: "latest",
								From: &kapi.ObjectReference{
									Kind: "DockerImage",
									Name: "index.docker.io/fedora:latest",
								},
							},
							"rawhide": {
								Name: "rawhide",
								From: &kapi.ObjectReference{
									Kind: "DockerImage",
									Name: "index.docker.io/fedora:rawhide",
								},
							},
							"unrelated": {
								Name: "rawhide",
								From: &kapi.ObjectReference{
									Kind: "DockerImage",
									Name: "docker.io/centos:foo",
								},
							},
						},
					},
				},
			},
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: true,
				Repository: &imageapi.RepositoryImportSpec{
					From: kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "docker.io/library/fedora",
					},
				},
			},
			expectedSpecRefs:   maxTestImportTagsPerRepository - 2,
			expectedStatusRefs: maxTestImportTagsPerRepository - 2,
		},

		{
			name: "import images",
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: true,
				Images: []imageapi.ImageImportSpec{
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/library/fedora:f23",
						},
					},
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/library/fedora",
						},
					},
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/centos:latest",
						},
					},
					{ // duplicate
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "index.docker.io/centos",
						},
					},
					{ // digest reference
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "index.docker.io/centos@" + imagetest.BaseImageWith1LayerDigest,
						},
					},
					{ // duplicate image stream image
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "local.docker.mirror.io/centos@" + imagetest.BaseImageWith1LayerDigest,
						},
					},
				},
			},
			expectedSpecRefs:   5,
			expectedStatusRefs: 4,
		},

		{
			name: "import image and repository",
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: true,
				Images: []imageapi.ImageImportSpec{
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/centos:latest",
						},
					},
				},
				Repository: &imageapi.RepositoryImportSpec{
					From: kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "docker.io/library/fedora",
					},
				},
			},
			expectedSpecRefs:   maxTestImportTagsPerRepository + 1,
			expectedStatusRefs: maxTestImportTagsPerRepository + 1,
		},

		{
			name: "import images and repository with overlapping reference",
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: true,
				Images: []imageapi.ImageImportSpec{
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/fedora:rawhide",
						},
					},
				},
				Repository: &imageapi.RepositoryImportSpec{
					From: kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "docker.io/library/fedora",
					},
				},
			},
			expectedSpecRefs:   maxTestImportTagsPerRepository,
			expectedStatusRefs: maxTestImportTagsPerRepository,
		},

		{
			name: "import images and repository with too many overlapping references",
			isiSpec: imageapi.ImageStreamImportSpec{
				Import: true,
				Images: []imageapi.ImageImportSpec{
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/fedora:rawhide",
						},
					},
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/fedora:f23",
						},
					},
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/fedora:f22",
						},
					},
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/fedora:f21",
						},
					},
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/fedora:f20",
						},
					},
					{
						From: kapi.ObjectReference{
							Kind: "DockerImage",
							Name: "docker.io/fedora:f19",
						},
					},
				},
				Repository: &imageapi.RepositoryImportSpec{
					From: kapi.ObjectReference{
						Kind: "DockerImage",
						Name: "docker.io/library/fedora",
					},
				},
			},
			// count just the number of images
			expectedSpecRefs:   6,
			expectedStatusRefs: 6,
		},
	} {

		fakeClient := &testclient.Fake{}
		fakeClient.AddReactor("list", "imagestreams", imagetest.GetFakeImageStreamListHandler(t, tc.iss...))

		evaluator := NewImageStreamImportEvaluator(fakeClient, maxTestImportTagsPerRepository)

		isi := &imageapi.ImageStreamImport{
			ObjectMeta: kapi.ObjectMeta{
				Namespace: "test",
				Name:      "is",
			},
			Spec: tc.isiSpec,
		}

		usage := evaluator.Usage(isi)

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
