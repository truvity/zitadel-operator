//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha2 "github.com/truvity/zitadel-operator/api/v1alpha2"
)

func TestSAMLApp_WithProjectRef(t *testing.T) {
	ctx := context.Background()
	ts := time.Now().UnixMilli()
	projName := fmt.Sprintf("samlproj-%d", ts)
	appName := fmt.Sprintf("samlapp-%d", ts)

	// Create Project CR.
	proj := &zitadelv1alpha2.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projName,
			Namespace: "default",
		},
	}
	if err := k8sClient.Create(ctx, proj); err != nil {
		t.Fatalf("creating Project CR: %v", err)
	}

	var reconciledProj zitadelv1alpha2.Project
	waitForReady(t, ctx, client.ObjectKeyFromObject(proj), &reconciledProj, 30*time.Second)
	t.Logf("project ready: %s (id: %s)", projName, reconciledProj.Status.ProjectId)

	// Create SAMLApp CR referencing the project.
	// Use a minimal but valid SAML SP metadata XML.
	samlMetadata := `<?xml version="1.0" encoding="UTF-8"?>
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata"
  entityID="https://sp.example.com/saml/metadata">
  <md:SPSSODescriptor AuthnRequestsSigned="false" WantAssertionsSigned="true"
    protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <md:AssertionConsumerService
      Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"
      Location="https://sp.example.com/saml/acs"
      index="0" isDefault="true"/>
  </md:SPSSODescriptor>
</md:EntityDescriptor>`

	app := &zitadelv1alpha2.SAMLApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: "default",
		},
		Spec: zitadelv1alpha2.SAMLAppSpec{
			ProjectRef: &zitadelv1alpha2.ResourceRef{
				Name: projName,
			},
			MetadataXml: samlMetadata,
		},
	}
	if err := k8sClient.Create(ctx, app); err != nil {
		t.Fatalf("creating SAMLApp CR: %v", err)
	}

	var reconciledApp zitadelv1alpha2.SAMLApp
	waitForReady(t, ctx, client.ObjectKeyFromObject(app), &reconciledApp, 60*time.Second)

	t.Logf("SAMLApp reconciled: appId=%s, projectId=%s, ready=%v",
		reconciledApp.Status.ApplicationId,
		reconciledApp.Status.ProjectId, reconciledApp.Status.Ready)

	if reconciledApp.Status.ApplicationId == "" {
		t.Fatal("expected applicationId to be set")
	}
	if reconciledApp.Status.ProjectId != reconciledProj.Status.ProjectId {
		t.Fatalf("expected projectId=%s, got %s", reconciledProj.Status.ProjectId, reconciledApp.Status.ProjectId)
	}

	// Cleanup.
	if err := k8sClient.Delete(ctx, app); err != nil {
		t.Fatalf("deleting SAMLApp CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(app), &zitadelv1alpha2.SAMLApp{}, 30*time.Second)

	if err := k8sClient.Delete(ctx, proj); err != nil {
		t.Fatalf("deleting Project CR: %v", err)
	}
	waitForDeletion(t, ctx, client.ObjectKeyFromObject(proj), &zitadelv1alpha2.Project{}, 30*time.Second)
	t.Log("SAMLApp with projectRef lifecycle complete")
}
