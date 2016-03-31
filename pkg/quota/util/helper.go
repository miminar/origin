package util

import (
	"strings"

	apierrs "k8s.io/kubernetes/pkg/api/errors"
)

// errMessageString is a part of error message copied from quotaAdmission.Admit() method in
// k8s.io/kubernetes/plugin/pkg/admission/resourcequota/admission.go module
const errMessageString = `exceeded quota:`

// IsErrorQuotaExceeded returns true if the given error stands for a denied request caused by detected quota
// abuse.
func IsErrorQuotaExceeded(err error) bool {
	return apierrs.IsForbidden(err) && strings.Contains(strings.ToLower(err.Error()), errMessageString)
}
