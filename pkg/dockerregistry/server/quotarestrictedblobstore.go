// Package server wraps repository and blob store objects of docker/distribution upstream. Most significantly,
// the wrappers cause manifests to be stored in OpenShift's etcd store instead of registry's storage.
// Registry's middleware API is utilized to register the object factories.
//
// Module with quotaRestrictedBlobStore defines a wrapper for upstream blob store that does an image quota
// check before committing image layer to a registry. Master server contains admission check that will refuse
// the manifest if the image exceeds whatever quota set. But the check occurs too late (after the layers are
// written). This addition allows us to refuse the layers and thus keep the storage clean.
//
// There are few things to keep in mind:
//
//   1. Origin master calculates image sizes from the contents of the layers. Registry, on the other hand,
//      deals with layers themselves that contain some overhead required to store file attributes, and the
//      layers are compressed. Thus we compare apples with pears. The check is primarily useful when the quota
//      is already almost reached.
//
//   2. During a push, multiple layers are uploaded. Uploaded layer does not raise the usage of any resource.
//      The usage will be raised during admission check once the manifest gets uploaded.
//
//   3. Here, we take into account just a single layer, not the image as a whole because the layers are
//      uploaded before the manifest.
//
//      This leads to a situation where several layers can be written until a big enough layer will be
//      received that exceeds quota limit.
//
//   3. Image stream size quota doesn't accumulate. Iow, its usage is NOT permanently stored in a resource
//      quota object. It's updated just for a very short period of time between an ImageStreamMapping object
//      is allowed by admission plugin to be created and subsequent quota refresh triggered by resource quota
//      controller. Therefore its check will probably not ever trigger unless uploaded layer is really big. We
//      could compute the usage here from corresponding image stream. We don't do so to keep the push
//      efficient.
package server

import (
	"fmt"
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"strings"

	kapi "k8s.io/kubernetes/pkg/api"
	kerrors "k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/quota"

	imageadmission "github.com/openshift/origin/pkg/image/admission"
	imageapi "github.com/openshift/origin/pkg/image/api"
)

// quotaRestrictedBlobStore wraps upstream blob store with a guard preventing big layers exceeding image quotas
// from being saved.
type quotaRestrictedBlobStore struct {
	distribution.BlobStore

	repo *repository
}

var _ distribution.BlobStore = &quotaRestrictedBlobStore{}

// Create wraps returned blobWriter with quota guard wrapper.
func (bs *quotaRestrictedBlobStore) Create(ctx context.Context) (distribution.BlobWriter, error) {
	context.GetLogger(ctx).Debug("(*quotaRestrictedBlobStore).Create: starting")

	bw, err := bs.BlobStore.Create(ctx)
	if err != nil {
		return nil, err
	}

	repo := (*bs.repo)
	repo.ctx = ctx
	return &quotaRestrictedBlobWriter{
		BlobWriter: bw,
		repo:       &repo,
	}, nil
}

// Resume wraps returned blobWriter with quota guard wrapper.
func (bs *quotaRestrictedBlobStore) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	context.GetLogger(ctx).Debug("(*quotaRestrictedBlobStore).Resume: starting")

	bw, err := bs.BlobStore.Resume(ctx, id)
	if err != nil {
		return nil, err
	}

	repo := (*bs.repo)
	repo.ctx = ctx
	return &quotaRestrictedBlobWriter{
		BlobWriter: bw,
		repo:       &repo,
	}, nil
}

// quotaRestrictedBlobWriter wraps upstream blob writer with a guard preventig big layers exceeding image
// quotas from being written.
type quotaRestrictedBlobWriter struct {
	distribution.BlobWriter

	repo *repository
}

func (bw *quotaRestrictedBlobWriter) Commit(ctx context.Context, provisional distribution.Descriptor) (canonical distribution.Descriptor, err error) {
	context.GetLogger(ctx).Debug("(*quotaRestrictedBlobWriter).Commit: starting")

	if err := admitBlobWrite(ctx, bw.repo, provisional.Size); err != nil {
		context.GetLogger(ctx).Error(err.Error())
		return distribution.Descriptor{}, err
	}

	can, err := bw.BlobWriter.Commit(ctx, provisional)
	return can, err
}

// admitBlobWrite checks whether the blob does not exceed image quota or limit ranges, if set. Returns
// ErrAccessDenied error if the quota is exceeded.
func admitBlobWrite(ctx context.Context, repo *repository, size int64) error {
	if err := admitLimitRanges(ctx, repo, size); err != nil {
		return err
	}

	if err := admitQuotas(ctx, repo); err != nil {
		return err
	}

	return nil
}

// admitLimitRanges checks the blob against any established limit ranges.
func admitLimitRanges(ctx context.Context, repo *repository, size int64) error {
	if size < 1 {
		return nil
	}

	limitranges, err := repo.limitClient.LimitRanges(repo.namespace).List(kapi.ListOptions{})
	if err != nil {
		if kerrors.IsForbidden(err) {
			context.GetLogger(ctx).Warnf("Cannot list limitranges because of outdated cluster roles: %v", err)
			return nil
		}
		context.GetLogger(ctx).Errorf("Failed to list limitranges: %v", err)
		return err
	}

	for _, limitrange := range limitranges.Items {
		for _, limit := range limitrange.Spec.Limits {
			if err := imageadmission.AdmitImage(size, limit); err != nil {
				context.GetLogger(ctx).Errorf("Refusing to write blob exceeding limit range: %s", err.Error())
				return distribution.ErrAccessDenied
			}
		}
	}

	return nil
}

// admitQuotas checks the blob against any established quotas.
func admitQuotas(ctx context.Context, repo *repository) error {
	rqs, err := repo.quotaClient.ResourceQuotas(repo.namespace).List(kapi.ListOptions{})
	if err != nil {
		if kerrors.IsForbidden(err) {
			context.GetLogger(ctx).Warnf("Cannot list resourcequotas because of outdated cluster roles: %v", err)
			return nil
		}
		context.GetLogger(ctx).Errorf("Failed to list resourcequotas: %v", err)
		return err
	}

	usage := kapi.ResourceList{
		// We're just checking whether we won't end up over the quota limit on image stream images. We don't
		// increment the usage here. It will be incremented during a creation of ImageStreamMapping caused by
		// manifest put.
		imageapi.ResourceImageStreamImages: *resource.NewQuantity(1, resource.DecimalSI),
	}
	resources := quota.ResourceNames(usage)

	for _, rq := range rqs.Items {
		newUsage := quota.Add(usage, rq.Status.Used)
		newUsage = quota.Mask(newUsage, resources)
		requested := quota.Mask(rq.Spec.Hard, resources)

		allowed, exceeded := quota.LessThanOrEqual(newUsage, requested)
		if !allowed {
			details := make([]string, len(exceeded))
			by := quota.Subtract(newUsage, requested)
			for i, r := range exceeded {
				details[i] = fmt.Sprintf("%s limited to %s by %s", r, requested[r], by[r])
			}
			context.GetLogger(ctx).Error("Refusing to write blob exceeding quota: " + strings.Join(details, ", "))
			return distribution.ErrAccessDenied
		}
	}

	return nil
}
