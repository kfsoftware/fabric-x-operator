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
	// CommitterValidatorFinalizerName is the name of the finalizer used by CommitterValidator
	CommitterValidatorFinalizerName = "committervalidator.fabricx.kfsoft.tech/finalizer"
)

// CommitterValidatorReconciler reconciles a CommitterValidator object
type CommitterValidatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committervalidators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committervalidators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committervalidators/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CommitterValidatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in CommitterValidator reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the CommitterValidator status to failed
			committerValidator := &fabricxv1alpha1.CommitterValidator{}
			if err := r.Get(ctx, req.NamespacedName, committerValidator); err == nil {
				panicMsg := fmt.Sprintf("Panic in CommitterValidator reconciliation: %v", panicErr)
				r.updateCommitterValidatorStatus(ctx, committerValidator, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var committerValidator fabricxv1alpha1.CommitterValidator
	if err := r.Get(ctx, req.NamespacedName, &committerValidator); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the CommitterValidator is being deleted
	if !committerValidator.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &committerValidator)
	}

	// Set initial status if not set
	if committerValidator.Status.Status == "" {
		r.updateCommitterValidatorStatus(ctx, &committerValidator, fabricxv1alpha1.PendingStatus, "Initializing CommitterValidator")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &committerValidator); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateCommitterValidatorStatus(ctx, &committerValidator, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the CommitterValidator
	if err := r.reconcileCommitterValidator(ctx, &committerValidator); err != nil {
		// The reconcileCommitterValidator method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if committerValidator.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile CommitterValidator: %v", err)
			r.updateCommitterValidatorStatus(ctx, &committerValidator, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile CommitterValidator")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute to ensure continuous monitoring
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileCommitterValidator handles the reconciliation of a CommitterValidator
func (r *CommitterValidatorReconciler) reconcileCommitterValidator(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	log := logf.FromContext(ctx)

	log.Info("Starting CommitterValidator reconciliation",
		"name", committerValidator.Name,
		"namespace", committerValidator.Namespace,
		"bootstrapMode", committerValidator.Spec.BootstrapMode)

	// Check bootstrap mode - only deploy when bootstrapMode is "deploy"
	bootstrapMode := committerValidator.Spec.BootstrapMode
	if bootstrapMode == "" {
		bootstrapMode = "configure" // Default to configure mode
	}

	// Reconcile based on deployment mode
	switch committerValidator.Spec.BootstrapMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, committerValidator); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateCommitterValidatorStatus(ctx, committerValidator, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, committerValidator); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateCommitterValidatorStatus(ctx, committerValidator, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid bootstrap mode: %s", committerValidator.Spec.BootstrapMode)
		log.Error(fmt.Errorf("%s", errorMsg), "Invalid bootstrap mode")
		r.updateCommitterValidatorStatus(ctx, committerValidator, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}

	// Update status to success
	r.updateCommitterValidatorStatus(ctx, committerValidator, fabricxv1alpha1.RunningStatus, "CommitterValidator reconciled successfully")

	log.Info("CommitterValidator reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only configuration resources)
func (r *CommitterValidatorReconciler) reconcileConfigureMode(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling CommitterValidator in configure mode",
		"name", committerValidator.Name,
		"namespace", committerValidator.Namespace)

	// In configure mode, only create certificates
	if err := r.reconcileCertificates(ctx, committerValidator); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}
	log.Info("CommitterValidator certificates created in configure mode")

	log.Info("CommitterValidator configure mode reconciliation completed")
	return nil
}

// reconcileCertificates creates or updates certificates for the CommitterValidator
func (r *CommitterValidatorReconciler) reconcileCertificates(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	log := logf.FromContext(ctx)

	// Check if enrollment is configured
	if committerValidator.Spec.Enrollment == nil {
		log.Info("No enrollment configuration found, skipping certificate creation")
		return nil
	}

	// Generate certificates for each type (each function handles its own existence check)
	var allCertificates []certs.ComponentCertificateData

	// Create sign certificate with component-specific SANS if available
	signCertConfig := &fabricxv1alpha1.CertificateConfig{
		CA: committerValidator.Spec.Enrollment.Sign.CA,
	}

	signRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    committerValidator.Name,
		ComponentType:    "validator",
		Namespace:        committerValidator.Namespace,
		OrdererGroupName: committerValidator.Name, // Using validator name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(committerValidator.Spec.MSPID, signCertConfig, "sign"),
		EnrollmentConfig: r.convertToEnrollmentConfig(committerValidator.Spec.MSPID, committerValidator.Spec.Enrollment),
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
		CA: committerValidator.Spec.Enrollment.TLS.CA,
	}
	// Use component-specific SANS if available, otherwise use enrollment SANS
	if committerValidator.Spec.SANS != nil {
		tlsCertConfig.SANS = committerValidator.Spec.SANS
	} else if committerValidator.Spec.Enrollment.TLS.SANS != nil {
		tlsCertConfig.SANS = committerValidator.Spec.Enrollment.TLS.SANS
	}

	tlsRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    committerValidator.Name,
		ComponentType:    "validator",
		Namespace:        committerValidator.Namespace,
		OrdererGroupName: committerValidator.Name, // Using validator name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(committerValidator.Spec.MSPID, tlsCertConfig, "tls"),
		EnrollmentConfig: r.convertToEnrollmentConfig(committerValidator.Spec.MSPID, committerValidator.Spec.Enrollment),
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
		if err := r.createCertificateSecrets(ctx, committerValidator, allCertificates); err != nil {
			return fmt.Errorf("failed to create certificate secrets: %w", err)
		}
	}

	log.Info("Certificates reconciled successfully", "validator", committerValidator.Name)
	return nil
}

// convertToCertConfig converts API certificate config to internal format
func (r *CommitterValidatorReconciler) convertToCertConfig(mspID string, apiConfig *fabricxv1alpha1.CertificateConfig, certType string) *certs.CertificateConfig {
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
func (r *CommitterValidatorReconciler) convertToEnrollmentConfig(mspID string, apiConfig *fabricxv1alpha1.EnrollmentConfig) *certs.EnrollmentConfig {
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
func (r *CommitterValidatorReconciler) createCertificateSecrets(
	ctx context.Context,
	committerValidator *fabricxv1alpha1.CommitterValidator,
	certificates []certs.ComponentCertificateData,
) error {
	log := logf.FromContext(ctx)

	// Process each certificate in the slice
	for _, certData := range certificates {
		secretName := fmt.Sprintf("%s-%s-cert", committerValidator.Name, certData.CertType)

		// Check if secret already exists
		existingSecret := &corev1.Secret{}
		err := r.Get(ctx, client.ObjectKey{
			Name:      secretName,
			Namespace: committerValidator.Namespace,
		}, existingSecret)

		if err != nil {
			if errors.IsNotFound(err) {
				// Secret doesn't exist, create it
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: committerValidator.Namespace,
						Labels: map[string]string{
							"app":                      "fabric-x",
							"committervalidator":       committerValidator.Name,
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
				if err := controllerutil.SetControllerReference(committerValidator, secret, r.Scheme); err != nil {
					return fmt.Errorf("failed to set controller reference for secret %s: %w", secretName, err)
				}

				if err := r.Create(ctx, secret); err != nil {
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
				"committervalidator":       committerValidator.Name,
				"certificate-type":         certData.CertType,
				"fabricx.kfsoft.tech/type": "certificate",
			}
			if !reflect.DeepEqual(existingSecret.Labels, expectedLabels) {
				updatedSecret.Labels = expectedLabels
				needsUpdate = true
			}

			if needsUpdate {
				if err := r.Update(ctx, updatedSecret); err != nil {
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
func (r *CommitterValidatorReconciler) reconcileDeployMode(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling CommitterValidator in deploy mode",
		"name", committerValidator.Name,
		"namespace", committerValidator.Namespace,
		"bootstrapMode", committerValidator.Spec.BootstrapMode)

	// Check if bootstrap mode is set to deploy
	if committerValidator.Spec.BootstrapMode != "deploy" {
		log.Info("Bootstrap mode is not 'deploy', skipping deployment resources",
			"bootstrapMode", committerValidator.Spec.BootstrapMode)
		return nil
	}

	// 1. Create/Update certificates first
	if err := r.reconcileCertificates(ctx, committerValidator); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}

	// 2. Create/Update Secret for Validator configuration
	if err := r.reconcileSecret(ctx, committerValidator); err != nil {
		return fmt.Errorf("failed to reconcile secret: %w", err)
	}

	// 3. Create/Update Service for Validator
	if err := r.reconcileService(ctx, committerValidator); err != nil {
		return fmt.Errorf("failed to reconcile service: %w", err)
	}

	// 4. Create/Update Deployment for Validator
	if err := r.reconcileDeployment(ctx, committerValidator); err != nil {
		return fmt.Errorf("failed to reconcile deployment: %w", err)
	}

	// 5. Create/Update Ingress for Validator (if configured)
	if committerValidator.Spec.Ingress != nil {
		if err := r.reconcileIngress(ctx, committerValidator); err != nil {
			return fmt.Errorf("failed to reconcile ingress: %w", err)
		}
	}

	log.Info("CommitterValidator deploy mode reconciliation completed")
	return nil
}

// reconcileConfigMap creates or updates the ConfigMap for Validator configuration
func (r *CommitterValidatorReconciler) reconcileSecret(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	log := logf.FromContext(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", committerValidator.Name),
			Namespace: committerValidator.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
	}

	// Prepare PostgreSQL template data
	postgresData := &utils.PostgreSQLTemplateData{
		Host:           "localhost",
		Port:           5435,
		Database:       "sc_db",
		Username:       "sc_user",
		Password:       "",
		MaxConnections: 80,
		MinConnections: 5,
		LoadBalance:    false,
		MaxElapsedTime: "1h",
	}

	// Override with configuration if provided
	if committerValidator.Spec.PostgreSQL != nil {
		if committerValidator.Spec.PostgreSQL.Host != "" {
			postgresData.Host = committerValidator.Spec.PostgreSQL.Host
		}
		if committerValidator.Spec.PostgreSQL.Port != 0 {
			postgresData.Port = committerValidator.Spec.PostgreSQL.Port
		}
		if committerValidator.Spec.PostgreSQL.Database != "" {
			postgresData.Database = committerValidator.Spec.PostgreSQL.Database
		}
		if committerValidator.Spec.PostgreSQL.Username != "" {
			postgresData.Username = committerValidator.Spec.PostgreSQL.Username
		}
		if committerValidator.Spec.PostgreSQL.MaxConnections != 0 {
			postgresData.MaxConnections = committerValidator.Spec.PostgreSQL.MaxConnections
		}
		if committerValidator.Spec.PostgreSQL.MinConnections != 0 {
			postgresData.MinConnections = committerValidator.Spec.PostgreSQL.MinConnections
		}
		postgresData.LoadBalance = committerValidator.Spec.PostgreSQL.LoadBalance
		if committerValidator.Spec.PostgreSQL.Retry != nil && committerValidator.Spec.PostgreSQL.Retry.MaxElapsedTime != "" {
			postgresData.MaxElapsedTime = committerValidator.Spec.PostgreSQL.Retry.MaxElapsedTime
		}
	}
	// If a secret is specified, fetch the password from the referenced secret
	if committerValidator.Spec.PostgreSQL != nil && committerValidator.Spec.PostgreSQL.PasswordSecret != nil {
		secretName := committerValidator.Spec.PostgreSQL.PasswordSecret.Name
		secretKey := committerValidator.Spec.PostgreSQL.PasswordSecret.Key
		secretNS := committerValidator.Spec.PostgreSQL.PasswordSecret.Namespace
		if secretName != "" && secretKey != "" {
			secret := &corev1.Secret{}
			if err := r.Get(ctx, client.ObjectKey{
				Name:      secretName,
				Namespace: secretNS,
			}, secret); err != nil {
				return fmt.Errorf("failed to get PostgreSQL password secret %s: %w", secretName, err)
			}
			if val, ok := secret.Data[secretKey]; ok {
				postgresData.Password = string(val)
			} else {
				return fmt.Errorf("key %s not found in secret %s", secretKey, secretName)
			}
		}
	}

	// Prepare template data
	templateData := utils.CommitterValidatorTemplateData{
		Name:       committerValidator.Name,
		PartyID:    committerValidator.Spec.PartyID,
		MSPID:      committerValidator.Spec.MSPID,
		Port:       6001, // Validator port
		PostgreSQL: postgresData,
	}

	// Execute the template using the shared utility
	configContent, err := utils.ExecuteTemplateWithValidation(utils.ValidatorConfigTemplate, templateData)
	if err != nil {
		return fmt.Errorf("failed to execute validator config template: %w", err)
	}

	template := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", committerValidator.Name),
			Namespace: committerValidator.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"validator_config.yaml": []byte(configContent),
		},
	}

	if err := r.updateSecret(ctx, committerValidator, secret, template); err != nil {
		log.Error(err, "Failed to update Secret", "name", secret.Name)
		return fmt.Errorf("failed to update Secret %s: %w", secret.Name, err)
	}

	log.Info("Secret reconciled successfully", "validator", committerValidator.Name)
	return nil
}

// updateSecret updates a secret with template data
func (r *CommitterValidatorReconciler) updateSecret(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator, secret *corev1.Secret, template *corev1.Secret) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(committerValidator, secret, r.Scheme); err != nil {
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

// reconcileService creates or updates the Service for Validator
func (r *CommitterValidatorReconciler) reconcileService(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", committerValidator.Name),
			Namespace: committerValidator.Namespace,
		},
	}

	template := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", committerValidator.Name),
			Namespace: committerValidator.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port:       6001,
					TargetPort: intstr.FromInt(6001),
					Protocol:   corev1.ProtocolTCP,
					Name:       "validator",
				},
			},
			Selector: map[string]string{
				"app":     "validator",
				"release": committerValidator.Name,
			},
		},
	}

	if err := r.updateService(ctx, committerValidator, service, template); err != nil {
		log.Error(err, "Failed to update Service", "name", service.Name)
		return fmt.Errorf("failed to update Service %s: %w", service.Name, err)
	}

	log.Info("Service reconciled successfully", "validator", committerValidator.Name)
	return nil
}

// updateService updates a service with template data
func (r *CommitterValidatorReconciler) updateService(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator, service *corev1.Service, template *corev1.Service) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(committerValidator, service, r.Scheme); err != nil {
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

// reconcileDeployment creates or updates the Deployment for Validator
func (r *CommitterValidatorReconciler) reconcileDeployment(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	log := logf.FromContext(ctx)

	// Compute combined hash of all mounted secrets/configmaps for rollout on change
	var hashParts []string

	// Hash config secret
	configSecretName := fmt.Sprintf("%s-config", committerValidator.Name)
	if hash, err := r.computeSecretHash(ctx, configSecretName, committerValidator.Namespace); err != nil {
		log.Error(err, "Failed to compute config secret hash", "secretName", configSecretName)
	} else {
		hashParts = append(hashParts, hash)
	}

	// Hash sign certificate secret
	signCertSecretName := fmt.Sprintf("%s-sign-cert", committerValidator.Name)
	if hash, err := r.computeSecretHash(ctx, signCertSecretName, committerValidator.Namespace); err != nil {
		log.V(1).Info("Sign cert secret not found or failed to hash", "secretName", signCertSecretName)
	} else {
		hashParts = append(hashParts, hash)
	}

	// Hash TLS certificate secret
	tlsCertSecretName := fmt.Sprintf("%s-tls-cert", committerValidator.Name)
	if hash, err := r.computeSecretHash(ctx, tlsCertSecretName, committerValidator.Namespace); err != nil {
		log.V(1).Info("TLS cert secret not found or failed to hash", "secretName", tlsCertSecretName)
	} else {
		hashParts = append(hashParts, hash)
	}

	// Combine all hashes
	sort.Strings(hashParts)
	combinedHashSum := sha256.Sum256([]byte(strings.Join(hashParts, "|")))
	configMapHash := hex.EncodeToString(combinedHashSum[:])

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      committerValidator.Name,
			Namespace: committerValidator.Namespace,
		},
	}

	// Prepare environment variables
	envVars := []corev1.EnvVar{}

	// Add custom environment variables
	for _, env := range committerValidator.Spec.Env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
	}

	template := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      committerValidator.Name,
			Namespace: committerValidator.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &committerValidator.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "validator",
					"release": committerValidator.Name,
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "validator",
						"release": committerValidator.Name,
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
							Name: "validator",
							Image: fmt.Sprintf("%s:%s",
								func() string {
									if committerValidator.Spec.Image != "" {
										return committerValidator.Spec.Image
									}
									return "hyperledger/fabric-x-committer"
								}(),
								func() string {
									if committerValidator.Spec.ImageTag != "" {
										return committerValidator.Spec.ImageTag
									}
									return "0.1.9"
								}()),
							Command: []string{
								"committer",
							},
							Args: []string{
								"start-vc",
								"--config=/etc/hyperledger/fabricx/validator/validator_config.yaml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "valid-port",
									ContainerPort: 6001,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: envVars,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									ReadOnly:  true,
									MountPath: "/etc/hyperledger/fabricx/validator",
								},
							},
							Resources: func() corev1.ResourceRequirements {
								if committerValidator.Spec.Resources != nil {
									return *committerValidator.Spec.Resources
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
									SecretName: fmt.Sprintf("%s-config", committerValidator.Name),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := r.updateDeployment(ctx, committerValidator, deployment, template); err != nil {
		log.Error(err, "Failed to update Deployment", "name", deployment.Name)
		return fmt.Errorf("failed to update Deployment %s: %w", deployment.Name, err)
	}

	log.Info("Deployment reconciled successfully", "validator", committerValidator.Name)
	return nil
}

// updateDeployment updates a deployment with template data
func (r *CommitterValidatorReconciler) updateDeployment(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator, deployment *appsv1.Deployment, template *appsv1.Deployment) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(committerValidator, deployment, r.Scheme); err != nil {
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
func (r *CommitterValidatorReconciler) computeSecretHash(ctx context.Context, secretName, namespace string) (string, error) {
	sec := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, sec); err != nil {
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

// reconcileIngress creates or updates the Ingress for Validator
func (r *CommitterValidatorReconciler) reconcileIngress(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	// TODO: Implement Ingress reconciliation
	// This would create/update an Ingress resource based on the ingress configuration
	return nil
}

// handleDeletion handles the deletion of a CommitterValidator
func (r *CommitterValidatorReconciler) handleDeletion(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Handling CommitterValidator deletion",
		"name", committerValidator.Name,
		"namespace", committerValidator.Namespace)

	// Set status to indicate deletion
	r.updateCommitterValidatorStatus(ctx, committerValidator, fabricxv1alpha1.PendingStatus, "Deleting CommitterValidator resources")

	// TODO: Clean up resources based on deployment mode
	// - Delete Deployments/StatefulSets
	// - Delete Services
	// - Delete PVCs
	// - Delete ConfigMaps and Secrets

	// Remove finalizer
	if err := r.removeFinalizer(ctx, committerValidator); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateCommitterValidatorStatus(ctx, committerValidator, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("CommitterValidator deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the CommitterValidator
func (r *CommitterValidatorReconciler) ensureFinalizer(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	if !utils.ContainsString(committerValidator.Finalizers, CommitterValidatorFinalizerName) {
		committerValidator.Finalizers = append(committerValidator.Finalizers, CommitterValidatorFinalizerName)
		return r.Update(ctx, committerValidator)
	}
	return nil
}

// removeFinalizer removes the finalizer from the CommitterValidator
func (r *CommitterValidatorReconciler) removeFinalizer(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator) error {
	committerValidator.Finalizers = utils.RemoveString(committerValidator.Finalizers, CommitterValidatorFinalizerName)
	return r.Update(ctx, committerValidator)
}

// updateCommitterValidatorStatus updates the CommitterValidator status with the given status and message
func (r *CommitterValidatorReconciler) updateCommitterValidatorStatus(ctx context.Context, committerValidator *fabricxv1alpha1.CommitterValidator, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating CommitterValidator status",
		"name", committerValidator.Name,
		"namespace", committerValidator.Namespace,
		"status", status,
		"message", message)

	// Update the status
	committerValidator.Status.Status = status
	committerValidator.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	committerValidator.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, committerValidator); err != nil {
		log.Error(err, "Failed to update CommitterValidator status")
	} else {
		log.Info("CommitterValidator status updated successfully",
			"name", committerValidator.Name,
			"namespace", committerValidator.Namespace,
			"status", status,
			"message", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CommitterValidatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.CommitterValidator{}).
		Named("committervalidator").
		Complete(r)
}
