package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/truvity/zitadel-operator/internal/config"
)

const (
	// ConditionTypeProjectScope indicates project scope label validation status.
	ConditionTypeProjectScope = "ProjectScopeMismatch"
)

// validateProjectScope checks that the CRD's namespace has the expected project scope label.
// When projectScopeLabel is empty, always returns shouldProceed=true (no enforcement).
// When configured but the label is missing, returns shouldProceed=false (fail-closed)
// and sets a condition with MissingLabel reason.
// When the label is present, returns shouldProceed=true and sets a condition with LabelPresent reason.
func validateProjectScope(
	ctx context.Context,
	k8s client.Client,
	cfg *config.Config,
	namespace string,
	conditions *[]metav1.Condition,
) (shouldProceed bool, err error) {
	if cfg.ProjectScopeLabel == "" {
		return true, nil
	}

	// Fetch the namespace object.
	var ns corev1.Namespace
	if err := k8s.Get(ctx, types.NamespacedName{Name: namespace}, &ns); err != nil {
		return false, fmt.Errorf("get namespace %q: %w", namespace, err)
	}

	labelValue, ok := ns.Labels[cfg.ProjectScopeLabel]
	if !ok {
		// Fail-closed: label missing.
		setCondition(conditions, ConditionTypeProjectScope, metav1.ConditionFalse,
			"MissingLabel",
			fmt.Sprintf("namespace %q missing required label %q", namespace, cfg.ProjectScopeLabel))
		return false, nil
	}

	// Label is present — validation passes.
	// Clear any previous ProjectScopeMismatch condition by setting it to True.
	setCondition(conditions, ConditionTypeProjectScope, metav1.ConditionTrue,
		"LabelPresent",
		fmt.Sprintf("namespace %q has label %q=%q", namespace, cfg.ProjectScopeLabel, labelValue))

	return true, nil
}
