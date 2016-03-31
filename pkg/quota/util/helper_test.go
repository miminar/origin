package util

import (
	"errors"
	"testing"

	kapi "k8s.io/kubernetes/pkg/api"
	kerrors "k8s.io/kubernetes/pkg/api/errors"
)

// TestIsErrorQuotaExceeded verifies that if a resource exceedes allowed usage, the admission will return
// error we can recognize.
func TestIsErrorQuotaExceeded(t *testing.T) {
	for _, tc := range []struct {
		name        string
		err         error
		shouldMatch bool
	}{
		{
			name: "unrelated error",
			err:  errors.New("unrelated"),
		},
		{
			name: "wrong type",
			err:  errors.New(errMessageString),
		},
		{
			name: "wrong kapi type",
			err:  kerrors.NewUnauthorized(errMessageString),
		},
		{
			name: "unrelated forbidden error",
			err:  kerrors.NewForbidden(kapi.Resource("imageStreams"), "is", errors.New("unrelated")),
		},
		{
			name:        "quota exceeded error",
			err:         kerrors.NewForbidden(kapi.Resource("imageStreams"), "is", errors.New(errMessageString)),
			shouldMatch: true,
		},
	} {
		match := IsErrorQuotaExceeded(tc.err)

		if !match && tc.shouldMatch {
			t.Errorf("[%s] expected to match error [%T]: %v", tc.name, tc.err, tc.err)
		}
		if match && !tc.shouldMatch {
			t.Errorf("[%s] expected not to match error [%T]: %v", tc.name, tc.err, tc.err)
		}
	}
}
