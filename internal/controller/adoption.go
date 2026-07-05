package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
)

// regenerateAdoptedClientSecret recovers the client secret for an adopted application.
//
// When an existing Zitadel application is found by name instead of created (e.g. a
// recreated CR adopts an app orphaned by an unclean teardown), the client secret
// cannot be read back from the API. If the referenced Secret already holds a
// non-empty client secret we keep it (no needless rotation) and return "".
// Otherwise a fresh secret is generated via the Zitadel API and returned so the
// caller can store it.
func regenerateAdoptedClientSecret(ctx context.Context, k8s client.Client, apps applicationv2.ApplicationServiceClient,
	namespace, secretName, secretKey, projectID, appID string) (string, error) {
	secret := &corev1.Secret{}
	err := k8s.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return "", fmt.Errorf("checking credential secret: %w", err)
	}
	if err == nil && len(secret.Data[secretKey]) > 0 {
		// Secret already holds a client secret — keep current behavior, do not rotate.
		return "", nil
	}

	logger := log.FromContext(ctx)
	logger.Info("adopted application has no stored client secret, regenerating",
		"appId", appID, "secret", secretName, "key", secretKey)

	resp, genErr := apps.GenerateClientSecret(ctx, &applicationv2.GenerateClientSecretRequest{
		ApplicationId: appID,
		ProjectId:     projectID,
	})
	if genErr != nil {
		return "", fmt.Errorf("regenerating client secret for adopted application: %w", genErr)
	}
	return resp.GetClientSecret(), nil
}
