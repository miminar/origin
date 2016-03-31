package images

import (
	"fmt"
	"os"
	"strconv"
	"time"

	kapi "k8s.io/kubernetes/pkg/api"
	kerrors "k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/resource"
	kclient "k8s.io/kubernetes/pkg/client/unversioned"
	kutilerrors "k8s.io/kubernetes/pkg/util/errors"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	imageapi "github.com/openshift/origin/pkg/image/api"
	exutil "github.com/openshift/origin/test/extended/util"
)

const (
	imageSize = 100

	quotaName = "image-quota"

	waitTimeout = time.Second * 30
)

var _ = g.Describe("[images] openshift image quota admission limiting image references", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("resourcequota-admission", exutil.KubeConfigPath())

	g.JustBeforeEach(func() {
		g.By("Waiting for builder service account")
		err := exutil.WaitForBuilderAccount(oc.KubeREST().ServiceAccounts(oc.Namespace()))
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// needs to be run at the of of each It; cannot be run in AfterEach which is run after the project
	// is destroyed
	tearDown := func(oc *exutil.CLI) {
		g.By(fmt.Sprintf("Deleting quota %s", quotaName))
		oc.AdminKubeREST().ResourceQuotas(oc.Namespace()).Delete(quotaName)

		deleteTestImagesAndStreams(oc)
	}

	g.It("should deny a push of image exceeding quota", func() {
		oc.SetOutputDir(exutil.TestContext.OutputDir)
		defer tearDown(oc)

		hard := kapi.ResourceList{
			imageapi.ResourceImageStreamTags:   resource.MustParse("0"),
			imageapi.ResourceImageStreamImages: resource.MustParse("0"),
		}
		_, err := createResourceQuota(oc, hard)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push image exceeding quota %v", hard))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "sized", "refused", imageSize, 2, false)
		o.Expect(err).NotTo(o.HaveOccurred())

		hard, err = bumpQuota(oc, imageapi.ResourceImageStreamImages, 1)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push image below quota %v", hard))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "sized", "first", imageSize, 2, true)
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err := waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push image exceeding quota %v", hard))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "sized", "second", imageSize, 2, false)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push image exceeding quota %v to another repository", hard))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "other", "third", imageSize, 2, false)
		o.Expect(err).NotTo(o.HaveOccurred())

		hard, err = bumpQuota(oc, imageapi.ResourceImageStreamImages, 2)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push image below quota %v", hard))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "other", "second", imageSize, 2, true)
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err = waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push image exceeding quota %v", hard))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "other", "refused", imageSize, 2, false)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push image exceeding quota %v to a new repository", hard))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "new", "refused", imageSize, 2, false)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("removing image sized:first")
		err = oc.REST().ImageStreamTags(oc.Namespace()).Delete("sized", "first")
		o.Expect(err).NotTo(o.HaveOccurred())
		// expect usage decrement
		used, err = exutil.WaitForResourceQuotaSync(
			oc.KubeREST().ResourceQuotas(oc.Namespace()),
			quotaName,
			kapi.ResourceList{imageapi.ResourceImageStreamImages: resource.MustParse("1")},
			true,
			time.Second*5)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push image below %s=%d quota", imageapi.ResourceImageStreamImages, 2))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "sized", "foo", imageSize, 2, true)
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err = waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())
	})

	g.It(fmt.Sprintf("should deny a tagging of an image exceeding %s quota", imageapi.ResourceImageStreamTags), func() {
		oc.SetOutputDir(exutil.TestContext.OutputDir)
		defer tearDown(oc)

		sharedProjectName, _, err := buildTestImagesInSharedNamespace(oc, 2)
		o.Expect(err).NotTo(o.HaveOccurred())

		hard := kapi.ResourceList{imageapi.ResourceImageStreamTags: resource.MustParse("0")}
		_, err = createResourceQuota(oc, hard)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to tag an image exceeding %v", hard))
		out, err := oc.Run("tag").Args(sharedProjectName+"/src:tag1", "is:tag1").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).Should(o.MatchRegexp("(?i)exceeded quota"))
		o.Expect(out).Should(o.ContainSubstring(string(imageapi.ResourceImageStreamTags)))

		hard, err = bumpQuota(oc, imageapi.ResourceImageStreamTags, 1)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to tag an image below quota %v", hard))
		out, err = oc.Run("tag").Args(sharedProjectName+"/src:tag1", "is:tag1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err := waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to tag an image exceeding quota %v", hard))
		out, err = oc.Run("tag").Args(sharedProjectName+"/src:tag2", "is:tag2").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).Should(o.MatchRegexp("(?i)exceeded quota"))
		o.Expect(out).Should(o.ContainSubstring(string(imageapi.ResourceImageStreamTags)))

		g.By("re-tagging the image under different tag")
		out, err = oc.Run("tag").Args(sharedProjectName+"/src:tag1", "is:1again").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err = waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		hard, err = bumpQuota(oc, imageapi.ResourceImageStreamTags, 2)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to alias tag a second image below quota %v", hard))
		out, err = oc.Run("tag").Args("--alias", "--source=istag", sharedProjectName+"/src:tag2", "other:tag2").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err = waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		g.By("re-tagging the image under different tag")
		out, err = oc.Run("tag").Args("--alias", "--source=istag", sharedProjectName+"/src:tag2", "another:2again").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err = waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("re-tagging another image should exceed quota %v", hard))
		out, err = oc.Run("tag").Args("--alias", "--source=istag", sharedProjectName+"/src:tag1", "another:1again").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).Should(o.MatchRegexp("(?i)exceeded quota"))
		o.Expect(out).Should(o.ContainSubstring(string(imageapi.ResourceImageStreamTags)))
	})

	g.It(fmt.Sprintf("should deny a tagging of an image exceeding %s quota using istag", imageapi.ResourceImageStreamTags), func() {
		oc.SetOutputDir(exutil.TestContext.OutputDir)
		defer tearDown(oc)

		sharedProjectName, tag2Image, err := buildTestImagesInSharedNamespace(oc, 3)
		o.Expect(err).NotTo(o.HaveOccurred())

		hard := kapi.ResourceList{imageapi.ResourceImageStreamTags: resource.MustParse("1")}
		_, err = createResourceQuota(oc, hard)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to create ImageStreamTag referencing isimage below quota %v", hard))
		ist := &imageapi.ImageStreamTag{
			ObjectMeta: kapi.ObjectMeta{
				Name: "dest:tag1",
			},
			Tag: &imageapi.TagReference{
				Name: "1",
				From: &kapi.ObjectReference{
					Kind:      "ImageStreamImage",
					Namespace: sharedProjectName,
					Name:      "src@" + tag2Image["tag1"].Name,
				},
			},
		}
		ist, err = oc.REST().ImageStreamTags(oc.Namespace()).Update(ist)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to create ImageStreamTag referencing isimage exceeding quota %v", hard))
		ist = &imageapi.ImageStreamTag{
			ObjectMeta: kapi.ObjectMeta{
				Name: "dest:tag2",
			},
			Tag: &imageapi.TagReference{
				Name: "2",
				From: &kapi.ObjectReference{
					Kind:      "ImageStreamImage",
					Namespace: sharedProjectName,
					Name:      "src@" + tag2Image["tag2"].Name,
				},
			},
		}
		_, err = oc.REST().ImageStreamTags(oc.Namespace()).Update(ist)
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.MatchRegexp("(?i)exceeded quota"))

		g.By("trying to create ImageStreamTag referencing isimage already referenced")
		ist = &imageapi.ImageStreamTag{
			ObjectMeta: kapi.ObjectMeta{
				Name: "dest:tag1again",
			},
			Tag: &imageapi.TagReference{
				Name: "tag1again",
				From: &kapi.ObjectReference{
					Kind:      "ImageStreamImage",
					Namespace: sharedProjectName,
					Name:      "src@" + tag2Image["tag1"].Name,
				},
			},
		}
		_, err = oc.REST().ImageStreamTags(oc.Namespace()).Update(ist)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = bumpQuota(oc, imageapi.ResourceImageStreamTags, 2)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to create ImageStreamTag referencing istag below quota %v", hard))
		ist = &imageapi.ImageStreamTag{
			ObjectMeta: kapi.ObjectMeta{
				Name: "dest:tag2",
			},
			Tag: &imageapi.TagReference{
				Name: "2",
				From: &kapi.ObjectReference{
					Kind:      "ImageStreamTag",
					Namespace: sharedProjectName,
					Name:      "src:tag2",
				},
			},
		}
		ist, err = oc.REST().ImageStreamTags(oc.Namespace()).Update(ist)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to create ImageStreamTag referencing istag exceeding quota %v", hard))
		ist = &imageapi.ImageStreamTag{
			ObjectMeta: kapi.ObjectMeta{
				Name: "dest:tag3",
			},
			Tag: &imageapi.TagReference{
				Name: "3",
				From: &kapi.ObjectReference{
					Kind:      "ImageStreamTag",
					Namespace: sharedProjectName,
					Name:      "src:tag3",
				},
			},
		}
		_, err = oc.REST().ImageStreamTags(oc.Namespace()).Update(ist)
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.MatchRegexp("(?i)exceeded quota"))

		g.By("trying to create ImageStreamTag referencing istag already referenced")
		ist = &imageapi.ImageStreamTag{
			ObjectMeta: kapi.ObjectMeta{
				Name: "dest:tag2again",
			},
			Tag: &imageapi.TagReference{
				Name: "tag2again",
				From: &kapi.ObjectReference{
					Kind:      "ImageStreamTag",
					Namespace: sharedProjectName,
					Name:      "src:tag2",
				},
			},
		}
		_, err = oc.REST().ImageStreamTags(oc.Namespace()).Update(ist)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It(fmt.Sprintf("should deny a docker image reference exceeding %s quota", imageapi.ResourceImageStreamTags), func() {
		oc.SetOutputDir(exutil.TestContext.OutputDir)
		defer tearDown(oc)

		sharedProjectName, tag2Image, err := buildTestImagesInSharedNamespace(oc, 2)
		o.Expect(err).NotTo(o.HaveOccurred())

		hard := kapi.ResourceList{imageapi.ResourceImageStreamTags: resource.MustParse("0")}
		_, err = createResourceQuota(oc, hard)
		o.Expect(err).NotTo(o.HaveOccurred())

		// image importer needs to be given permissions to pull from another namespace if referencing with
		// DockerImage
		err = permitPullsFromNamespace(oc, sharedProjectName, oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to tag a docker image exceeding quota %v", hard))
		out, err := oc.Run("import-image").Args("stream:dockerimage", "--confirm", "--insecure", "--from", tag2Image["tag1"].DockerImageReference).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).Should(o.MatchRegexp("(?i)exceeded quota"))

		hard, err = bumpQuota(oc, imageapi.ResourceImageStreamTags, 1)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to tag a docker image below quota %v", hard))
		err = oc.Run("import-image").Args("stream:dockerimage", "--confirm", "--insecure", "--from", tag2Image["tag1"].DockerImageReference).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, "stream", "dockerimage")
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err := waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to tag a docker image exceeding quota %v", hard))
		is, err := oc.REST().ImageStreams(oc.Namespace()).Get("stream")
		o.Expect(err).NotTo(o.HaveOccurred())
		is.Spec.Tags["foo"] = imageapi.TagReference{
			Name: "foo",
			From: &kapi.ObjectReference{
				Kind: "DockerImage",
				Name: tag2Image["tag2"].DockerImageReference,
			},
			ImportPolicy: imageapi.TagImportPolicy{
				Insecure: true,
			},
		}
		_, err = oc.REST().ImageStreams(oc.Namespace()).Update(is)
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(kerrors.IsForbidden(err)).To(o.Equal(true))

		g.By("re-tagging the image under different tag")
		is, err = oc.REST().ImageStreams(oc.Namespace()).Get("stream")
		o.Expect(err).NotTo(o.HaveOccurred())
		is.Spec.Tags["duplicate"] = imageapi.TagReference{
			Name: "duplicate",
			From: &kapi.ObjectReference{
				Kind: "DockerImage",
				Name: tag2Image["tag1"].DockerImageReference,
			},
			ImportPolicy: imageapi.TagImportPolicy{
				Insecure: true,
			},
		}
		_, err = oc.REST().ImageStreams(oc.Namespace()).Update(is)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, "stream", "duplicate")
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err = waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())
	})

	g.It(fmt.Sprintf("should deny an import of a repository exceeding %s quota", imageapi.ResourceImageStreamTags), func() {
		oc.SetOutputDir(exutil.TestContext.OutputDir)
		defer tearDown(oc)

		maxBulkImport, err := getMaxImagesBulkImportedPerRepository()
		o.Expect(err).NotTo(o.HaveOccurred())

		s1Name, s1tag2Image, err := buildTestImagesInSharedNamespaceWithSuffix(oc, "-s1", maxBulkImport+1)
		s2Name, s2tag2Image, err := buildTestImagesInSharedNamespaceWithSuffix(oc, "-s2", 2)
		o.Expect(err).NotTo(o.HaveOccurred())

		hard := kapi.ResourceList{
			imageapi.ResourceImageStreamTags:   *resource.NewQuantity(int64(maxBulkImport)+1, resource.DecimalSI),
			imageapi.ResourceImageStreamImages: *resource.NewQuantity(int64(maxBulkImport)+1, resource.DecimalSI),
		}
		_, err = createResourceQuota(oc, hard)
		o.Expect(err).NotTo(o.HaveOccurred())

		// image importer needs to be given permissions to pull from another namespace if referencing with
		// DockerImage
		err = permitPullsFromNamespace(oc, s1Name, oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())
		err = permitPullsFromNamespace(oc, s2Name, oc.Namespace())
		o.Expect(err).NotTo(o.HaveOccurred())

		s1ref, err := imageapi.ParseDockerImageReference(s1tag2Image["tag1"].DockerImageReference)
		o.Expect(err).NotTo(o.HaveOccurred())
		s1ref.Tag = ""
		s1ref.ID = ""
		s2ref, err := imageapi.ParseDockerImageReference(s2tag2Image["tag1"].DockerImageReference)
		o.Expect(err).NotTo(o.HaveOccurred())
		s2ref.Tag = ""
		s2ref.ID = ""

		g.By(fmt.Sprintf("trying to import from repository %q below quota %v", s1ref.Exact(), hard))
		err = oc.Run("import-image").Args("bulkimport", "--confirm", "--insecure", "--all", "--from", s1ref.Exact()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, "bulkimport", "tag1")
		o.Expect(err).NotTo(o.HaveOccurred())
		expected := kapi.ResourceList{
			imageapi.ResourceImageStreamTags: *resource.NewQuantity(int64(maxBulkImport), resource.DecimalSI),
			// it will take some time before the value is bumped to maxBulkImport*2 (untill the next quota
			// usage sync)
			imageapi.ResourceImageStreamImages: *resource.NewQuantity(int64(maxBulkImport), resource.DecimalSI),
		}
		used, err := waitForResourceQuotaSync(oc, quotaName, expected)
		o.Expect(err).NotTo(o.HaveOccurred())
		// we cannot be sure the quota is up to date for image stream images; check only the exact value of
		// image stream tags
		delete(expected, imageapi.ResourceImageStreamImages)
		delete(used, imageapi.ResourceImageStreamImages)
		o.Expect(assertQuotasEqual(used, expected)).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to import tags from repository %q exceeding quota %v", s2ref.Exact(), hard))
		out, err := oc.Run("import-image").Args("bulkimport", "--confirm", "--insecure", "--all", "--from", s2ref.Exact()).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).Should(o.MatchRegexp("(?i)exceeded quota"))
		o.Expect(out).Should(o.ContainSubstring(string(imageapi.ResourceImageStreamTags)))
	})

	g.It(fmt.Sprintf("should deny a tagging of an image exceeding %s quota", imageapi.ResourceImageStreamImages), func() {
		oc.SetOutputDir(exutil.TestContext.OutputDir)
		defer tearDown(oc)

		sharedProjectName, _, err := buildTestImagesInSharedNamespace(oc, 3)
		o.Expect(err).NotTo(o.HaveOccurred())

		hard := kapi.ResourceList{imageapi.ResourceImageStreamImages: resource.MustParse("0")}
		_, err = createResourceQuota(oc, hard)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to tag an image exceeding %v", hard))
		out, err := oc.Run("tag").Args(sharedProjectName+"/src:tag1", "is:tag1").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).Should(o.MatchRegexp("(?i)exceeded quota"))
		o.Expect(out).Should(o.ContainSubstring(string(imageapi.ResourceImageStreamImages)))

		hard, err = bumpQuota(oc, imageapi.ResourceImageStreamImages, 1)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to tag an image below quota %v", hard))
		err = oc.Run("tag").Args(sharedProjectName+"/src:tag1", "is:tag1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, "is", "tag1")
		o.Expect(err).NotTo(o.HaveOccurred())
		// image references in spec and status are different, but status will be added
		// during the next quota sync
		used, err := waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to tag an image exceeding quota %v", hard))
		out, err = oc.Run("tag").Args(sharedProjectName+"/src:tag2", "is:tag2").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(out).Should(o.MatchRegexp("(?i)exceeded quota"))
		o.Expect(out).Should(o.ContainSubstring(string(imageapi.ResourceImageStreamImages)))

		g.By("re-tagging the image under different tag")
		err = oc.Run("tag").Args(sharedProjectName+"/src:tag1", "is:1again").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, "is", "1again")
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err = waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		hard, err = bumpQuota(oc, imageapi.ResourceImageStreamImages, 2)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to alias tag a second image below quota %v", hard))
		err = oc.Run("tag").Args("--alias", "--source=istag", sharedProjectName+"/src:tag2", "other:tag2").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, "other", "tag2")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = forceQuotaResync(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err = waitForResourceQuotaSync(oc, quotaName, hard)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, hard)).NotTo(o.HaveOccurred())

		// an alias cannot be gated - no usage increment is detected for istag
		g.By("exceeding quota with another istag")
		err = oc.Run("tag").Args("--alias", "--source=istag", sharedProjectName+"/src:tag3", "is:tag3").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, "is", "tag3")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = forceQuotaResync(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		expected := kapi.ResourceList{imageapi.ResourceImageStreamImages: *resource.NewQuantity(3, resource.DecimalSI)}
		used, err = waitForResourceQuotaSync(oc, quotaName, expected)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, expected)).NotTo(o.HaveOccurred())

		// an alias cannot be gated - no usage increment is detected for istag
		g.By("re-tagging the image under different tag should still be allowed")
		err = oc.Run("tag").Args("--alias", "--source=istag", sharedProjectName+"/src:tag2", "another:2again").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForAnImageStreamTag(oc, "another", "2again")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = forceQuotaResync(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		used, err = waitForResourceQuotaSync(oc, quotaName, expected)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(assertQuotasEqual(used, expected)).NotTo(o.HaveOccurred())
	})
})

// buildTestImagesInSharedNamespaceWithSuffix creates a shared namespace derived from current project and
// builds a given number of test images. The images are pushed to the shared namespace into src image streams
// under tagX where X is a number of image starting from 1. The shared namespace's name is a current one plus
// given suffix.
func buildTestImagesInSharedNamespaceWithSuffix(oc *exutil.CLI, suffix string, numberOfImages int) (sharedProjectName string, tag2Image map[string]imageapi.Image, err error) {
	projectName := oc.Namespace()
	sharedProjectName = projectName + suffix
	g.By(fmt.Sprintf("Create a new project %s to store shared images", sharedProjectName))
	err = oc.Run("new-project").Args(sharedProjectName).Execute()
	if err != nil {
		return
	}
	oc.SetNamespace(sharedProjectName)
	defer oc.SetNamespace(projectName)

	g.By(fmt.Sprintf("Waiting for builder service account in namespace %s", sharedProjectName))
	err = exutil.WaitForBuilderAccount(oc.KubeREST().ServiceAccounts(sharedProjectName))
	o.Expect(err).NotTo(o.HaveOccurred())

	tag2Image = make(map[string]imageapi.Image)
	for i := 1; i <= numberOfImages; i++ {
		tag := fmt.Sprintf("tag%d", i)
		err = BuildAndPushImageOfSize(oc, sharedProjectName, "src", tag, imageSize, 2, true)
		if err != nil {
			return "", nil, err
		}
		ist, err := oc.REST().ImageStreamTags(sharedProjectName).Get("src", tag)
		if err != nil {
			return "", nil, err
		}
		tag2Image[tag] = ist.Image
	}

	g.By(fmt.Sprintf("Switch back to the original project %s", projectName))
	err = oc.Run("project").Args(projectName).Execute()
	if err != nil {
		return "", nil, err
	}
	return
}

// buildTestImagesInSharedNamespace creates a shared namespace derived from current project and builds a given
// number of test images. The images are pushed to the shared namespace into src image streams under tagX
// where X is a number of image starting from 1.
func buildTestImagesInSharedNamespace(oc *exutil.CLI, numberOfImages int) (sharedProjectName string, tag2Image map[string]imageapi.Image, err error) {
	return buildTestImagesInSharedNamespaceWithSuffix(oc, "-shared", numberOfImages)
}

// createResourceQuota creates a resource quota with given hard limits in a current namespace and waits until
// a first usage refresh
func createResourceQuota(oc *exutil.CLI, hard kapi.ResourceList) (*kapi.ResourceQuota, error) {
	rq := &kapi.ResourceQuota{
		ObjectMeta: kapi.ObjectMeta{
			Name: quotaName,
		},
		Spec: kapi.ResourceQuotaSpec{
			Hard: hard,
		},
	}

	g.By(fmt.Sprintf("creating resource quota with a limit %v", hard))
	rq, err := oc.AdminKubeREST().ResourceQuotas(oc.Namespace()).Create(rq)
	if err != nil {
		return nil, err
	}
	err = waitForLimitSync(oc, hard)
	return rq, err
}

// permitPullsFromNamespace gives privileges to service accounts in targetNamespace to pull images from
// sourceNamespace. This is needed when referencing images in the source namespace using docker image
// references.
func permitPullsFromNamespace(oc *exutil.CLI, sourceNamespace, targetNamespace string) error {
	if sourceNamespace == targetNamespace {
		return nil
	}
	originalNamespace := oc.Namespace()
	oc.SetNamespace(sourceNamespace)
	defer oc.SetNamespace(originalNamespace)

	return oc.Run("policy").Args("add-role-to-group", "system:image-puller", "system:serviceaccounts:"+targetNamespace).Execute()
}

// assertQuotasEqual compares two quota sets and returns an error with proper description when they don't match
func assertQuotasEqual(a, b kapi.ResourceList) error {
	errs := []error{}
	if len(a) != len(b) {
		errs = append(errs, fmt.Errorf("number of items does not match (%d != %d)", len(a), len(b)))
	}

	for k, av := range a {
		if bv, exists := b[k]; exists {
			if av.Cmp(bv) != 0 {
				errs = append(errs, fmt.Errorf("a[%s] != b[%s] (%s != %s)", k, k, av.String(), bv.String()))
			}
		} else {
			errs = append(errs, fmt.Errorf("resource %q not present in b", k))
		}
	}

	for k := range b {
		if _, exists := a[k]; !exists {
			errs = append(errs, fmt.Errorf("resource %q not present in a", k))
		}
	}

	return kutilerrors.NewAggregate(errs)
}

// bumpQuota modifies hard spec of quota object with the given value. It returns modified hard spec.
func bumpQuota(oc *exutil.CLI, resourceName kapi.ResourceName, value int64) (kapi.ResourceList, error) {
	g.By(fmt.Sprintf("bump the quota to %s=%d", resourceName, value))
	rq, err := oc.AdminKubeREST().ResourceQuotas(oc.Namespace()).Get(quotaName)
	if err != nil {
		return nil, err
	}
	rq.Spec.Hard[resourceName] = *resource.NewQuantity(value, resource.DecimalSI)
	_, err = oc.AdminKubeREST().ResourceQuotas(oc.Namespace()).Update(rq)
	if err != nil {
		return nil, err
	}
	err = waitForLimitSync(oc, rq.Spec.Hard)
	if err != nil {
		return nil, err
	}
	return rq.Spec.Hard, nil
}

// waitForResourceQuotaSync waits until a usage of a quota reaches given limit with a short timeout
func waitForResourceQuotaSync(oc *exutil.CLI, name string, expectedResources kapi.ResourceList) (kapi.ResourceList, error) {
	g.By(fmt.Sprintf("waiting for resource quota %s to get updated", name))
	used, err := exutil.WaitForResourceQuotaSync(
		oc.KubeREST().ResourceQuotas(oc.Namespace()),
		quotaName,
		expectedResources,
		false,
		waitTimeout,
	)
	if err != nil {
		return nil, err
	}
	return used, nil
}

// waitForAnImageStreamTag waits until an image stream with given name has non-empty history for given tag
func waitForAnImageStreamTag(oc *exutil.CLI, name, tag string) error {
	g.By(fmt.Sprintf("waiting for an is importer to import a tag %s into a stream %s", tag, name))
	start := time.Now()
	c := make(chan error)
	go func() {
		err := exutil.WaitForAnImageStream(
			oc.REST().ImageStreams(oc.Namespace()),
			name,
			func(is *imageapi.ImageStream) bool {
				if history, exists := is.Status.Tags[tag]; !exists || len(history.Items) == 0 {
					return false
				}
				return true
			},
			func(is *imageapi.ImageStream) bool {
				return time.Now().After(start.Add(waitTimeout))
			})
		c <- err
	}()

	select {
	case e := <-c:
		return e
	case <-time.After(waitTimeout):
		return fmt.Errorf("timed out while waiting of an image stream tag %s/%s:%s", oc.Namespace(), name, tag)
	}
}

// waitForResourceQuotaSync waits until a usage of a quota reaches given limit with a short timeout
func waitForLimitSync(oc *exutil.CLI, hardLimit kapi.ResourceList) error {
	g.By(fmt.Sprintf("waiting for resource quota %s to get updated", quotaName))
	return exutil.WaitForResourceQuotaLimitSync(
		oc.KubeREST().ResourceQuotas(oc.Namespace()),
		quotaName,
		hardLimit,
		waitTimeout)
}

// getMaxImagesBulkImportedPerRepository returns a maximum numbers of images that can be imported from
// repository at once. The value is obtained from environment variable which must be set.
func getMaxImagesBulkImportedPerRepository() (int, error) {
	max := os.Getenv("MAX_IMAGES_BULK_IMPORTED_PER_REPOSITORY")
	if len(max) == 0 {
		return 0, fmt.Errorf("MAX_IMAGES_BULK_IMAGES_IMPORTED_PER_REPOSITORY needs to be set")
	}
	return strconv.Atoi(max)
}

// deleteTestImagesAndStreams deletes test images built in current and shared namespaces.
// It also deletes shared projects.
func deleteTestImagesAndStreams(oc *exutil.CLI) {
	for _, projectName := range []string{
		oc.Namespace() + "-s2",
		oc.Namespace() + "-s1",
		oc.Namespace() + "-shared",
		oc.Namespace(),
	} {
		g.By(fmt.Sprintf("Deleting images and image streams in project %q", projectName))
		iss, err := oc.AdminREST().ImageStreams(projectName).List(kapi.ListOptions{})
		if err != nil {
			continue
		}
		for _, is := range iss.Items {
			for _, history := range is.Status.Tags {
				for i := range history.Items {
					oc.AdminREST().Images().Delete(history.Items[i].Image)
				}
			}
			for _, tagRef := range is.Spec.Tags {
				switch tagRef.From.Kind {
				case "ImageStreamImage":
					_, id, err := imageapi.ParseImageStreamImageName(tagRef.From.Name)
					if err != nil {
						continue
					}
					oc.AdminREST().Images().Delete(id)
				}
			}
			oc.AdminREST().ImageStreams(is.Namespace).Delete(is.Name)
		}

		// let the extended framework take care of the current namespace
		if projectName != oc.Namespace() {
			g.By(fmt.Sprintf("Deleting project %q", projectName))
			oc.AdminREST().Projects().Delete(projectName)
		}
	}
}

// forceQuotaSync modifies some unrelated resource hard limit in resource quota object in order to force quota
// usage resync.
func forceQuotaResync(oc *exutil.CLI) error {
	const syncAnnotation = "test.sync.counter"
	hard := kapi.ResourceList{}
	err := kclient.RetryOnConflict(kclient.DefaultRetry, func() error {
		rq, err := oc.AdminKubeREST().ResourceQuotas(oc.Namespace()).Get(quotaName)
		if err != nil {
			return err
		}
		quantity, ok := rq.Spec.Hard[kapi.ResourceQuotas]
		if !ok {
			quantity = *resource.NewQuantity(0, resource.DecimalSI)
		}
		quantity.Set(quantity.Value() + 1)
		rq.Spec.Hard[kapi.ResourceQuotas] = quantity
		_, err = oc.AdminKubeREST().ResourceQuotas(oc.Namespace()).Update(rq)
		hard = rq.Spec.Hard
		return err
	})
	if err != nil {
		return err
	}

	return waitForLimitSync(oc, hard)
}
