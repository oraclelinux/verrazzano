// Copyright (c) 2023, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package thanos

import (
	"fmt"
	"path/filepath"

	"github.com/verrazzano/verrazzano/pkg/k8s/ready"
	"github.com/verrazzano/verrazzano/pkg/vzcr"
	"github.com/verrazzano/verrazzano/platform-operator/constants"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/authproxy"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/helm"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/networkpolicies"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/nginx"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"github.com/verrazzano/verrazzano/platform-operator/internal/config"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ComponentName is the name of the component
const ComponentName = "thanos"

// ComponentNamespace is the namespace of the component
const ComponentNamespace = constants.VerrazzanoMonitoringNamespace

// ComponentJSONName is the JSON name of the Thanos component in CRD
const ComponentJSONName = "thanos"

// Availability Object Names
const (
	queryDeployment    = "thanos-query"
	frontendDeployment = "thanos-query"
)

type thanosComponent struct {
	helm.HelmComponent
}

func NewComponent() spi.Component {
	return thanosComponent{
		helm.HelmComponent{
			ReleaseName:               ComponentName,
			JSONName:                  ComponentJSONName,
			ChartDir:                  filepath.Join(config.GetThirdPartyDir(), ComponentName),
			ChartNamespace:            ComponentNamespace,
			IgnoreNamespaceOverride:   true,
			SupportsOperatorInstall:   true,
			SupportsOperatorUninstall: true,
			ImagePullSecretKeyname:    "image.pullSecrets[0]",
			ValuesFile:                filepath.Join(config.GetHelmOverridesDir(), "thanos-values.yaml"),
			Dependencies:              []string{networkpolicies.ComponentName, nginx.ComponentName},
			AppendOverridesFunc:       AppendOverrides,
			GetInstallOverridesFunc:   GetOverrides,
			AvailabilityObjects: &ready.AvailabilityObjects{
				DeploymentNames: []types.NamespacedName{
					{
						Name:      frontendDeployment,
						Namespace: ComponentNamespace,
					},
					{
						Name:      queryDeployment,
						Namespace: ComponentNamespace,
					},
				},
			},
		},
	}
}

// IsReady component check for Thanos
func (t thanosComponent) IsReady(ctx spi.ComponentContext) bool {
	return t.HelmComponent.IsReady(ctx) && t.isThanosReady(ctx)
}

// isThanosReady returns true if the availability objects have the minimum number of expected replicas
func (t thanosComponent) isThanosReady(ctx spi.ComponentContext) bool {
	prefix := fmt.Sprintf("Component %s", ctx.GetComponent())
	return ready.DeploymentsAreReady(ctx.Log(), ctx.Client(), t.AvailabilityObjects.DeploymentNames, 1, prefix)
}

// IsEnabled Thanos enabled check for installation
func (t thanosComponent) IsEnabled(effectiveCR runtime.Object) bool {
	return vzcr.IsThanosEnabled(effectiveCR)
}

// PreInstall handles the pre-install operations for the Thanos component
func (t thanosComponent) PreInstall(ctx spi.ComponentContext) error {
	if err := preInstallUpgrade(ctx); err != nil {
		return err
	}

	return t.HelmComponent.PreInstall(ctx)
}

// PreUpgrade handles the pre-upgrade operations for the Thanos component
func (t thanosComponent) PreUpgrade(ctx spi.ComponentContext) error {
	if err := preInstallUpgrade(ctx); err != nil {
		return err
	}

	return t.HelmComponent.PreUpgrade(ctx)
}

// PostInstall handles the post-install operations for the Thanos component
func (t thanosComponent) PostInstall(ctx spi.ComponentContext) error {
	if err := postInstallUpgrade(ctx); err != nil {
		return err
	}

	t.IngressNames = t.GetIngressNames(ctx)
	t.Certificates = t.GetCertificateNames(ctx)
	return t.HelmComponent.PostInstall(ctx)
}

// PostUpgrade handles the post-upgrade operations for the Thanos component
func (t thanosComponent) PostUpgrade(ctx spi.ComponentContext) error {
	if err := postInstallUpgrade(ctx); err != nil {
		return err
	}

	return t.HelmComponent.PostUpgrade(ctx)
}

// GetIngressNames returns the Thanos ingress names
func (t thanosComponent) GetIngressNames(ctx spi.ComponentContext) []types.NamespacedName {
	var ingressNames []types.NamespacedName
	if !vzcr.IsThanosEnabled(ctx.EffectiveCR()) || !vzcr.IsNGINXEnabled(ctx.EffectiveCR()) {
		return ingressNames
	}
	ns := constants.VerrazzanoSystemNamespace
	if vzcr.IsAuthProxyEnabled(ctx.EffectiveCR()) {
		ns = authproxy.ComponentNamespace
	}
	return append(ingressNames, types.NamespacedName{
		Namespace: ns,
		Name:      constants.ThanosQueryIngress,
	})
}

// GetCertificateNames returns the TLS secret for the Thanos component
func (t thanosComponent) GetCertificateNames(ctx spi.ComponentContext) []types.NamespacedName {
	var certificateNames []types.NamespacedName

	if !vzcr.IsThanosEnabled(ctx.EffectiveCR()) || !vzcr.IsNGINXEnabled(ctx.EffectiveCR()) {
		return certificateNames
	}
	ns := constants.VerrazzanoSystemNamespace
	if vzcr.IsAuthProxyEnabled(ctx.EffectiveCR()) {
		ns = authproxy.ComponentNamespace
	}
	return append(certificateNames, types.NamespacedName{
		Namespace: ns,
		Name:      queryCertificateName,
	})
}