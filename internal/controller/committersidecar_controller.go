/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

const (
	// CommitterSidecarFinalizerName is the name of the finalizer used by CommitterSidecar
	CommitterSidecarFinalizerName = "committersidecar.fabricx.kfsoft.tech/finalizer"
)

// CommitterSidecarReconciler reconciles a CommitterSidecar object
type CommitterSidecarReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committersidecars,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committersidecars/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committersidecars/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CommitterSidecarReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in CommitterSidecar reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the CommitterSidecar status to failed
			committerSidecar := &fabricxv1alpha1.CommitterSidecar{}
			if err := r.Get(ctx, req.NamespacedName, committerSidecar); err == nil {
				panicMsg := fmt.Sprintf("Panic in CommitterSidecar reconciliation: %v", panicErr)
				r.updateCommitterSidecarStatus(ctx, committerSidecar, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var committerSidecar fabricxv1alpha1.CommitterSidecar
	if err := r.Get(ctx, req.NamespacedName, &committerSidecar); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the CommitterSidecar is being deleted
	if !committerSidecar.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &committerSidecar)
	}

	// Set initial status if not set
	if committerSidecar.Status.Status == "" {
		r.updateCommitterSidecarStatus(ctx, &committerSidecar, fabricxv1alpha1.PendingStatus, "Initializing CommitterSidecar")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &committerSidecar); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateCommitterSidecarStatus(ctx, &committerSidecar, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the CommitterSidecar
	if err := r.reconcileCommitterSidecar(ctx, &committerSidecar); err != nil {
		// The reconcileCommitterSidecar method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if committerSidecar.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile CommitterSidecar: %v", err)
			r.updateCommitterSidecarStatus(ctx, &committerSidecar, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile CommitterSidecar")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute to ensure continuous monitoring
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileCommitterSidecar handles the reconciliation of a CommitterSidecar
func (r *CommitterSidecarReconciler) reconcileCommitterSidecar(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	log := logf.FromContext(ctx)

	log.Info("Starting CommitterSidecar reconciliation",
		"name", committerSidecar.Name,
		"namespace", committerSidecar.Namespace,
		"bootstrapMode", committerSidecar.Spec.BootstrapMode)

	// Check bootstrap mode - only deploy when bootstrapMode is "deploy"
	bootstrapMode := committerSidecar.Spec.BootstrapMode
	if bootstrapMode == "" {
		bootstrapMode = "configure" // Default to configure mode
	}

	// Reconcile based on deployment mode
	switch committerSidecar.Spec.BootstrapMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, committerSidecar); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateCommitterSidecarStatus(ctx, committerSidecar, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, committerSidecar); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateCommitterSidecarStatus(ctx, committerSidecar, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid bootstrap mode: %s", committerSidecar.Spec.BootstrapMode)
		log.Error(fmt.Errorf("%s", errorMsg), "Invalid bootstrap mode")
		r.updateCommitterSidecarStatus(ctx, committerSidecar, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}

	// Update status to success
	r.updateCommitterSidecarStatus(ctx, committerSidecar, fabricxv1alpha1.RunningStatus, "CommitterSidecar reconciled successfully")

	log.Info("CommitterSidecar reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only configuration resources)
func (r *CommitterSidecarReconciler) reconcileConfigureMode(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling CommitterSidecar in configure mode",
		"name", committerSidecar.Name,
		"namespace", committerSidecar.Namespace)

	// In configure mode, only create certificates
	if err := r.reconcileCertificates(ctx, committerSidecar); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}
	log.Info("CommitterSidecar certificates created in configure mode")

	log.Info("CommitterSidecar configure mode reconciliation completed")
	return nil
}

// reconcileCertificates creates or updates certificates for the CommitterSidecar
func (r *CommitterSidecarReconciler) reconcileCertificates(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	log := logf.FromContext(ctx)

	// Check if enrollment is configured
	if committerSidecar.Spec.Enrollment == nil {
		log.Info("No enrollment configuration found, skipping certificate creation")
		return nil
	}

	// Generate certificates for each type (each function handles its own existence check)
	var allCertificates []certs.ComponentCertificateData

	// Create sign certificate with component-specific SANS if available
	signCertConfig := &fabricxv1alpha1.CertificateConfig{
		CA: committerSidecar.Spec.Enrollment.Sign.CA,
	}

	signRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    committerSidecar.Name,
		ComponentType:    "sidecar",
		Namespace:        committerSidecar.Namespace,
		OrdererGroupName: committerSidecar.Name, // Using sidecar name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(committerSidecar.Spec.MSPID, signCertConfig, "sign"),
		EnrollmentConfig: r.convertToEnrollmentConfig(committerSidecar.Spec.MSPID, committerSidecar.Spec.Enrollment),
	}
	signCertData, err := certs.CreateSignCertificate(ctx, r.Client, signRequest)
	if err != nil {
		return fmt.Errorf("failed to create sign certificate: %w", err)
	}
	if signCertData != nil {
		allCertificates = append(allCertificates, *signCertData)
	}

	// Create TLS certificate with component-specific SANS if available
	tlsCertConfig := &fabricxv1alpha1.CertificateConfig{
		CA: committerSidecar.Spec.Enrollment.TLS.CA,
	}
	// Use component-specific SANS if available, otherwise use enrollment SANS
	if committerSidecar.Spec.SANS != nil {
		tlsCertConfig.SANS = committerSidecar.Spec.SANS
	} else if committerSidecar.Spec.Enrollment.TLS.SANS != nil {
		tlsCertConfig.SANS = committerSidecar.Spec.Enrollment.TLS.SANS
	}

	tlsRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    committerSidecar.Name,
		ComponentType:    "sidecar",
		Namespace:        committerSidecar.Namespace,
		OrdererGroupName: committerSidecar.Name, // Using sidecar name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(committerSidecar.Spec.MSPID, tlsCertConfig, "tls"),
		EnrollmentConfig: r.convertToEnrollmentConfig(committerSidecar.Spec.MSPID, committerSidecar.Spec.Enrollment),
	}
	tlsCertData, err := certs.CreateTLSCertificate(ctx, r.Client, tlsRequest)
	if err != nil {
		return fmt.Errorf("failed to create TLS certificate: %w", err)
	}
	if tlsCertData != nil {
		allCertificates = append(allCertificates, *tlsCertData)
	}

	// Create Kubernetes secrets for the certificates (only if any were generated)
	if len(allCertificates) > 0 {
		if err := r.createCertificateSecrets(ctx, committerSidecar, allCertificates); err != nil {
			return fmt.Errorf("failed to create certificate secrets: %w", err)
		}
	}

	log.Info("Certificates reconciled successfully", "sidecar", committerSidecar.Name)
	return nil
}

// convertToCertConfig converts API certificate config to internal format
func (r *CommitterSidecarReconciler) convertToCertConfig(mspID string, apiConfig *fabricxv1alpha1.CertificateConfig, certType string) *certs.CertificateConfig {
	if apiConfig == nil {
		return nil
	}

	config := &certs.CertificateConfig{
		MSPID: mspID,
	}

	// Add CA configuration if provided
	if apiConfig.CA != nil {
		config.CA = &certs.CACertificateConfig{
			CAHost:       apiConfig.CA.CAHost,
			CAName:       apiConfig.CA.CAName,
			CAPort:       apiConfig.CA.CAPort,
			EnrollID:     apiConfig.CA.EnrollID,
			EnrollSecret: apiConfig.CA.EnrollSecret,
		}

		// Add CATLS configuration if provided
		if apiConfig.CA.CATLS != nil {
			config.CA.CATLS = &certs.CATLSConfig{
				CACert: apiConfig.CA.CATLS.CACert,
			}
			if apiConfig.CA.CATLS.SecretRef != nil {
				config.CA.CATLS.SecretRef = &certs.SecretRef{
					Name:      apiConfig.CA.CATLS.SecretRef.Name,
					Key:       apiConfig.CA.CATLS.SecretRef.Key,
					Namespace: apiConfig.CA.CATLS.SecretRef.Namespace,
				}
			}
		}
	}

	// Add SANS configuration if provided
	if certType == "tls" && apiConfig.SANS != nil {
		config.SANS = &certs.SANSConfig{
			DNSNames:    apiConfig.SANS.DNSNames,
			IPAddresses: apiConfig.SANS.IPAddresses,
		}
	}

	return config
}

// convertToEnrollmentConfig converts API enrollment config to internal format
func (r *CommitterSidecarReconciler) convertToEnrollmentConfig(mspID string, apiConfig *fabricxv1alpha1.EnrollmentConfig) *certs.EnrollmentConfig {
	if apiConfig == nil {
		return nil
	}

	config := &certs.EnrollmentConfig{}

	if apiConfig.Sign != nil {
		config.Sign = r.convertToCertConfig(mspID, apiConfig.Sign, "sign")
	}

	if apiConfig.TLS != nil {
		config.TLS = r.convertToCertConfig(mspID, apiConfig.TLS, "tls")
	}

	return config
}

// createCertificateSecrets creates Kubernetes secrets for certificate data
func (r *CommitterSidecarReconciler) createCertificateSecrets(
	ctx context.Context,
	committerSidecar *fabricxv1alpha1.CommitterSidecar,
	certificates []certs.ComponentCertificateData,
) error {
	log := logf.FromContext(ctx)

	// Process each certificate in the slice
	for _, certData := range certificates {
		secretName := fmt.Sprintf("%s-%s-cert", committerSidecar.Name, certData.CertType)

		// Check if secret already exists
		existingSecret := &corev1.Secret{}
		err := r.Client.Get(ctx, client.ObjectKey{
			Name:      secretName,
			Namespace: committerSidecar.Namespace,
		}, existingSecret)

		if err != nil {
			if errors.IsNotFound(err) {
				// Secret doesn't exist, create it
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: committerSidecar.Namespace,
						Labels: map[string]string{
							"app":                      "fabric-x",
							"committersidecar":         committerSidecar.Name,
							"certificate-type":         certData.CertType,
							"fabricx.kfsoft.tech/type": "certificate",
						},
					},
					Data: map[string][]byte{
						"cert.pem": certData.Cert,
						"key.pem":  certData.Key,
						"ca.pem":   certData.CACert,
					},
				}

				// Set the controller reference
				if err := controllerutil.SetControllerReference(committerSidecar, secret, r.Scheme); err != nil {
					return fmt.Errorf("failed to set controller reference for secret %s: %w", secretName, err)
				}

				if err := r.Client.Create(ctx, secret); err != nil {
					return fmt.Errorf("failed to create certificate secret %s: %w", secretName, err)
				}

				log.Info("Created certificate secret", "secret", secretName, "certType", certData.CertType)
			} else {
				return fmt.Errorf("failed to check existing certificate secret %s: %w", secretName, err)
			}
		} else {
			// Secret exists, check if it needs to be updated
			needsUpdate := false
			updatedSecret := existingSecret.DeepCopy()

			// Check if certificate data has changed
			if !reflect.DeepEqual(existingSecret.Data["cert.pem"], certData.Cert) ||
				!reflect.DeepEqual(existingSecret.Data["key.pem"], certData.Key) ||
				!reflect.DeepEqual(existingSecret.Data["ca.pem"], certData.CACert) {

				updatedSecret.Data = map[string][]byte{
					"cert.pem": certData.Cert,
					"key.pem":  certData.Key,
					"ca.pem":   certData.CACert,
				}
				needsUpdate = true
			}

			// Check if labels need to be updated
			expectedLabels := map[string]string{
				"app":                      "fabric-x",
				"committersidecar":         committerSidecar.Name,
				"certificate-type":         certData.CertType,
				"fabricx.kfsoft.tech/type": "certificate",
			}
			if !reflect.DeepEqual(existingSecret.Labels, expectedLabels) {
				updatedSecret.Labels = expectedLabels
				needsUpdate = true
			}

			if needsUpdate {
				if err := r.Client.Update(ctx, updatedSecret); err != nil {
					return fmt.Errorf("failed to update certificate secret %s: %w", secretName, err)
				}
				log.Info("Updated certificate secret", "secret", secretName, "certType", certData.CertType)
			} else {
				log.Info("Certificate secret already exists and is up to date", "secret", secretName, "certType", certData.CertType)
			}
		}
	}

	return nil
}

// reconcileDeployMode handles reconciliation in deploy mode (full deployment)
func (r *CommitterSidecarReconciler) reconcileDeployMode(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling CommitterSidecar in deploy mode",
		"name", committerSidecar.Name,
		"namespace", committerSidecar.Namespace,
		"bootstrapMode", committerSidecar.Spec.BootstrapMode)

	// Check if bootstrap mode is set to deploy
	if committerSidecar.Spec.BootstrapMode != "deploy" {
		log.Info("Bootstrap mode is not 'deploy', skipping deployment resources",
			"bootstrapMode", committerSidecar.Spec.BootstrapMode)
		return nil
	}

	// 1. Create/Update certificates first
	if err := r.reconcileCertificates(ctx, committerSidecar); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}

	// 2. Create/Update genesis block secret
	if err := r.reconcileGenesisBlock(ctx, committerSidecar); err != nil {
		return fmt.Errorf("failed to reconcile genesis block: %w", err)
	}

	// 3. Create/Update Secret for Sidecar configuration
	if err := r.reconcileSecret(ctx, committerSidecar); err != nil {
		return fmt.Errorf("failed to reconcile secret: %w", err)
	}

	// 4. Create/Update Service for Sidecar
	if err := r.reconcileService(ctx, committerSidecar); err != nil {
		return fmt.Errorf("failed to reconcile service: %w", err)
	}

	// 5. Create/Update Deployment for Sidecar
	if err := r.reconcileDeployment(ctx, committerSidecar); err != nil {
		return fmt.Errorf("failed to reconcile deployment: %w", err)
	}

	// 6. Create/Update Ingress for Sidecar (if configured)
	if committerSidecar.Spec.Ingress != nil {
		if err := r.reconcileIngress(ctx, committerSidecar); err != nil {
			return fmt.Errorf("failed to reconcile ingress: %w", err)
		}
	}

	log.Info("CommitterSidecar deploy mode reconciliation completed")
	return nil
}

// reconcileConfigMap creates or updates the ConfigMap for Sidecar configuration
func (r *CommitterSidecarReconciler) reconcileSecret(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	log := logf.FromContext(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", committerSidecar.Name),
			Namespace: committerSidecar.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
	}

	// Prepare template data
	// Try to derive endpoints from the parent Committer CRD if provided
	// For now, use component endpoints if present; otherwise leave empty
	templateData := utils.CommitterSidecarTemplateData{
		Name:             committerSidecar.Name,
		PartyID:          committerSidecar.Spec.PartyID,
		MSPID:            committerSidecar.Spec.MSPID,
		Port:             5050,
		OrdererEndpoints: committerSidecar.Spec.OrdererEndpoints,
		CommitterHost:    committerSidecar.Spec.CommitterHost,
		CommitterPort:    committerSidecar.Spec.CommitterPort,
	}

	// Execute the template using the shared utility
	configContent, err := utils.ExecuteTemplateWithValidation(utils.SidecarConfigTemplate, templateData)
	if err != nil {
		return fmt.Errorf("failed to execute sidecar config template: %w", err)
	}

	template := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", committerSidecar.Name),
			Namespace: committerSidecar.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"sidecar_config.yaml": []byte(configContent),
		},
	}

	if err := r.updateSecret(ctx, committerSidecar, secret, template); err != nil {
		log.Error(err, "Failed to update Secret", "name", secret.Name)
		return fmt.Errorf("failed to update Secret %s: %w", secret.Name, err)
	}

	log.Info("Secret reconciled successfully", "sidecar", committerSidecar.Name)
	return nil
}

// updateSecret updates a secret with template data
func (r *CommitterSidecarReconciler) updateSecret(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar, secret *corev1.Secret, template *corev1.Secret) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(committerSidecar, secret, r.Scheme); err != nil {
			return err
		}

		// Update secret data
		secret.Type = template.Type
		secret.Data = template.Data

		// Update metadata
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			secret.Labels[k] = v
		}
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			secret.Annotations[k] = v
		}

		return nil
	})

	return err
}

// reconcileService creates or updates the Service for Sidecar
func (r *CommitterSidecarReconciler) reconcileService(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", committerSidecar.Name),
			Namespace: committerSidecar.Namespace,
		},
	}

	template := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", committerSidecar.Name),
			Namespace: committerSidecar.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port:       5050,
					TargetPort: intstr.FromInt(5050),
					Protocol:   corev1.ProtocolTCP,
					Name:       "sidecar",
				},
			},
			Selector: map[string]string{
				"app":     "sidecar",
				"release": committerSidecar.Name,
			},
		},
	}

	if err := r.updateService(ctx, committerSidecar, service, template); err != nil {
		log.Error(err, "Failed to update Service", "name", service.Name)
		return fmt.Errorf("failed to update Service %s: %w", service.Name, err)
	}

	log.Info("Service reconciled successfully", "sidecar", committerSidecar.Name)
	return nil
}

// updateService updates a service with template data
func (r *CommitterSidecarReconciler) updateService(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar, service *corev1.Service, template *corev1.Service) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(committerSidecar, service, r.Scheme); err != nil {
			return err
		}

		// Update service spec
		service.Spec = template.Spec

		// Update metadata
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			service.Labels[k] = v
		}
		if service.Annotations == nil {
			service.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			service.Annotations[k] = v
		}

		return nil
	})

	return err
}

// reconcileDeployment creates or updates the Deployment for Sidecar
func (r *CommitterSidecarReconciler) reconcileDeployment(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	log := logf.FromContext(ctx)

	// Compute Secret hash to trigger deployment updates when config changes
	configMapHash := ""
	configSecretName := fmt.Sprintf("%s-config", committerSidecar.Name)
	if hash, err := r.computeSecretHash(ctx, configSecretName, committerSidecar.Namespace); err != nil {
		log.Error(err, "Failed to compute Secret hash, continuing without hash", "secretName", configSecretName, "namespace", committerSidecar.Namespace)
	} else {
		configMapHash = hash
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      committerSidecar.Name,
			Namespace: committerSidecar.Namespace,
		},
	}

	template := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      committerSidecar.Name,
			Namespace: committerSidecar.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &committerSidecar.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "sidecar",
					"release": committerSidecar.Name,
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "sidecar",
						"release": committerSidecar.Name,
					},
					Annotations: func() map[string]string {
						annotations := make(map[string]string)
						if configMapHash != "" {
							annotations["fabricx.kfsoft.tech/config-hash"] = configMapHash
						}
						return annotations
					}(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "sidecar",
							Image: "hyperledger/fabric-x-committer:0.1.4",
							Command: []string{
								"committer",
							},
							Args: []string{
								"start-sidecar",
								"--config=/etc/hyperledger/fabricx/sidecar/sidecar_config.yaml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "sidecar-port",
									ContainerPort: 5050,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									ReadOnly:  true,
									MountPath: "/etc/hyperledger/fabricx/sidecar",
								},
								{
									Name:      "data",
									ReadOnly:  false,
									MountPath: "/var/hyperledger/fabricx/ledger",
								},
								{
									Name:      "genesis-block",
									ReadOnly:  true,
									MountPath: "/etc/hyperledger/fabricx/genesis",
								},
							},
							Resources: func() corev1.ResourceRequirements {
								if committerSidecar.Spec.Resources != nil {
									return *committerSidecar.Spec.Resources
								}
								return corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("500m"),
										corev1.ResourceMemory: resource.MustParse("512Mi"),
									},
								}
							}(),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-config", committerSidecar.Name),
								},
							},
						},
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "genesis-block",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: committerSidecar.Spec.Genesis.SecretName,
									Items: []corev1.KeyToPath{
										{
											Key: func() string {
												if committerSidecar.Spec.Genesis.SecretKey != "" {
													return committerSidecar.Spec.Genesis.SecretKey
												}
												return "genesis.block"
											}(),
											Path: "genesis.block",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := r.updateDeployment(ctx, committerSidecar, deployment, template); err != nil {
		log.Error(err, "Failed to update Deployment", "name", deployment.Name)
		return fmt.Errorf("failed to update Deployment %s: %w", deployment.Name, err)
	}

	log.Info("Deployment reconciled successfully", "sidecar", committerSidecar.Name)
	return nil
}

// updateDeployment updates a deployment with template data
func (r *CommitterSidecarReconciler) updateDeployment(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar, deployment *appsv1.Deployment, template *appsv1.Deployment) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(committerSidecar, deployment, r.Scheme); err != nil {
			return err
		}

		// Update deployment spec
		deployment.Spec = template.Spec

		// Update metadata
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			deployment.Labels[k] = v
		}
		if deployment.Annotations == nil {
			deployment.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			deployment.Annotations[k] = v
		}

		return nil
	})

	return err
}

// computeConfigMapHash computes a deterministic hash of a ConfigMap's data
func (r *CommitterSidecarReconciler) computeSecretHash(ctx context.Context, secretName, namespace string) (string, error) {
	sec := &corev1.Secret{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, sec); err != nil {
		return "", err
	}
	var parts []string
	for k, v := range sec.Data {
		parts = append(parts, fmt.Sprintf("%s=%s", k, string(v)))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:]), nil
}

// reconcileIngress creates or updates the Ingress for Sidecar
func (r *CommitterSidecarReconciler) reconcileIngress(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	// TODO: Implement Ingress reconciliation
	// This would create/update an Ingress resource based on the ingress configuration
	return nil
}

// reconcileGenesisBlock creates or updates the genesis block secret for the CommitterSidecar
func (r *CommitterSidecarReconciler) reconcileGenesisBlock(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	log := logf.FromContext(ctx)

	// Check if genesis configuration is provided
	if committerSidecar.Spec.Genesis.SecretName == "" {
		log.Info("No genesis block configuration found, skipping genesis block reconciliation")
		return nil
	}

	// Verify that the genesis block secret exists
	genesisSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: func() string {
			if committerSidecar.Spec.Genesis.SecretNamespace != "" {
				return committerSidecar.Spec.Genesis.SecretNamespace
			}
			return committerSidecar.Namespace
		}(),
		Name: committerSidecar.Spec.Genesis.SecretName,
	}, genesisSecret)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "Genesis block secret not found",
				"secretName", committerSidecar.Spec.Genesis.SecretName,
				"secretNamespace", func() string {
					if committerSidecar.Spec.Genesis.SecretNamespace != "" {
						return committerSidecar.Spec.Genesis.SecretNamespace
					}
					return committerSidecar.Namespace
				}())
			return fmt.Errorf("genesis block secret not found: %w", err)
		}
		return fmt.Errorf("failed to get genesis block secret: %w", err)
	}

	// Check if the genesis block data exists in the secret
	genesisKey := committerSidecar.Spec.Genesis.SecretKey
	if genesisKey == "" {
		genesisKey = "genesis.block" // Default key name
	}

	if _, exists := genesisSecret.Data[genesisKey]; !exists {
		log.Error(fmt.Errorf("genesis block data not found in secret"),
			"Genesis block data not found in secret",
			"secretName", committerSidecar.Spec.Genesis.SecretName,
			"secretKey", genesisKey)
		return fmt.Errorf("genesis block data not found in secret %s with key %s", committerSidecar.Spec.Genesis.SecretName, genesisKey)
	}

	log.Info("Genesis block secret verified successfully",
		"secretName", committerSidecar.Spec.Genesis.SecretName,
		"secretKey", genesisKey)
	return nil
}

// handleDeletion handles the deletion of a CommitterSidecar
func (r *CommitterSidecarReconciler) handleDeletion(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Handling CommitterSidecar deletion",
		"name", committerSidecar.Name,
		"namespace", committerSidecar.Namespace)

	// Set status to indicate deletion
	r.updateCommitterSidecarStatus(ctx, committerSidecar, fabricxv1alpha1.PendingStatus, "Deleting CommitterSidecar resources")

	// TODO: Clean up resources based on deployment mode
	// - Delete Deployments/StatefulSets
	// - Delete Services
	// - Delete PVCs
	// - Delete ConfigMaps and Secrets

	// Remove finalizer
	if err := r.removeFinalizer(ctx, committerSidecar); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateCommitterSidecarStatus(ctx, committerSidecar, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("CommitterSidecar deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the CommitterSidecar
func (r *CommitterSidecarReconciler) ensureFinalizer(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	if !utils.ContainsString(committerSidecar.Finalizers, CommitterSidecarFinalizerName) {
		committerSidecar.Finalizers = append(committerSidecar.Finalizers, CommitterSidecarFinalizerName)
		return r.Update(ctx, committerSidecar)
	}
	return nil
}

// removeFinalizer removes the finalizer from the CommitterSidecar
func (r *CommitterSidecarReconciler) removeFinalizer(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar) error {
	committerSidecar.Finalizers = utils.RemoveString(committerSidecar.Finalizers, CommitterSidecarFinalizerName)
	return r.Update(ctx, committerSidecar)
}

// updateCommitterSidecarStatus updates the CommitterSidecar status with the given status and message
func (r *CommitterSidecarReconciler) updateCommitterSidecarStatus(ctx context.Context, committerSidecar *fabricxv1alpha1.CommitterSidecar, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating CommitterSidecar status",
		"name", committerSidecar.Name,
		"namespace", committerSidecar.Namespace,
		"status", status,
		"message", message)

	// Update the status
	committerSidecar.Status.Status = status
	committerSidecar.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	committerSidecar.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, committerSidecar); err != nil {
		log.Error(err, "Failed to update CommitterSidecar status")
	} else {
		log.Info("CommitterSidecar status updated successfully",
			"name", committerSidecar.Name,
			"namespace", committerSidecar.Namespace,
			"status", status,
			"message", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CommitterSidecarReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.CommitterSidecar{}).
		Named("committersidecar").
		Complete(r)
}
