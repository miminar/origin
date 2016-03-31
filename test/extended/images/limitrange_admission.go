package images

import (
	"fmt"

	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	imageapi "github.com/openshift/origin/pkg/image/api"
	exutil "github.com/openshift/origin/test/extended/util"
)

const (
	limitRangeName = "limits"
)

var _ = g.Describe("[images] openshift image quota admission limiting image size", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("limitrange-admission", exutil.KubeConfigPath())

	g.JustBeforeEach(func() {
		g.By("Waiting for builder service account")
		err := exutil.WaitForBuilderAccount(oc.KubeREST().ServiceAccounts(oc.Namespace()))
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// needs to be run at the of of each It; cannot be run in AfterEach which is run after the project
	// is destroyed
	tearDown := func(oc *exutil.CLI) {
		g.By(fmt.Sprintf("Deleting limit range %s", limitRangeName))
		oc.AdminKubeREST().LimitRanges(oc.Namespace()).Delete(limitRangeName)

		deleteTestImagesAndStreams(oc)
	}

	g.It("should deny a push of built image exceeding size limit", func() {
		oc.SetOutputDir(exutil.TestContext.OutputDir)
		defer tearDown(oc)

		_, err := limitImageSize(oc, "10Ki")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push an image exceeding size limit with just 1 layer"))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "sized", "middle", 16000, 1, false)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push an image exceeding size limit in total"))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "sized", "middle", 16000, 5, false)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push an image with one big layer below size limit"))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "sized", "small", 8000, 1, true)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("trying to push an image below size limit"))
		err = BuildAndPushImageOfSize(oc, oc.Namespace(), "sized", "small", 8000, 2, true)
		o.Expect(err).NotTo(o.HaveOccurred())
	})
})

// limitImageSize creates a new limit range object with given max limit for image size in current namespace
func limitImageSize(oc *exutil.CLI, size string) (*kapi.LimitRange, error) {
	lr := &kapi.LimitRange{
		ObjectMeta: kapi.ObjectMeta{
			Name: limitRangeName,
		},
		Spec: kapi.LimitRangeSpec{
			Limits: []kapi.LimitRangeItem{
				{
					Type: imageapi.LimitTypeImageSize,
					Max: kapi.ResourceList{
						kapi.ResourceStorage: resource.MustParse(size),
					},
				},
			},
		},
	}

	g.By(fmt.Sprintf("creating limit range object %q with %s limited to %s", limitRangeName, imageapi.LimitTypeImageSize, size))
	lr, err := oc.AdminKubeREST().LimitRanges(oc.Namespace()).Create(lr)
	return lr, err
}
