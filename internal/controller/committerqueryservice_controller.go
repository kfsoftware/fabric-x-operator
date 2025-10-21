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
	// CommitterQueryServiceFinalizerName is the name of the finalizer used by CommitterQueryService
	CommitterQueryServiceFinalizerName = "committerqueryservice.fabricx.kfsoft.tech/finalizer"
)

// CommitterQueryServiceReconciler reconciles a CommitterQueryService object
type CommitterQueryServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committerqueryservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committerqueryservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=committerqueryservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CommitterQueryServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in CommitterQueryService reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the CommitterQueryService status to failed
			committerCoordinator := &fabricxv1alpha1.CommitterQueryService{}
			if err := r.Get(ctx, req.NamespacedName, committerCoordinator); err == nil {
				panicMsg := fmt.Sprintf("Panic in CommitterQueryService reconciliation: %v", panicErr)
				r.updateCommitterQueryServiceStatus(ctx, committerCoordinator, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var committerCoordinator fabricxv1alpha1.CommitterQueryService
	if err := r.Get(ctx, req.NamespacedName, &committerCoordinator); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the CommitterQueryService is being deleted
	if !committerCoordinator.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &committerCoordinator)
	}

	// Set initial status if not set
	if committerCoordinator.Status.Status == "" {
		r.updateCommitterQueryServiceStatus(ctx, &committerCoordinator, fabricxv1alpha1.PendingStatus, "Initializing CommitterQueryService")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &committerCoordinator); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateCommitterQueryServiceStatus(ctx, &committerCoordinator, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the CommitterQueryService
	if err := r.reconcileCommitterQueryService(ctx, &committerCoordinator); err != nil {
		// The reconcileCommitterQueryService method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if committerCoordinator.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile CommitterQueryService: %v", err)
			r.updateCommitterQueryServiceStatus(ctx, &committerCoordinator, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile CommitterQueryService")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute to ensure continuous monitoring
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileCommitterQueryService handles the reconciliation of a CommitterQueryService
func (r *CommitterQueryServiceReconciler) reconcileCommitterQueryService(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	log := logf.FromContext(ctx)

	log.Info("Starting CommitterQueryService reconciliation",
		"name", committerCoordinator.Name,
		"namespace", committerCoordinator.Namespace,
		"bootstrapMode", committerCoordinator.Spec.BootstrapMode)

	// Check bootstrap mode - only deploy when bootstrapMode is "deploy"
	bootstrapMode := committerCoordinator.Spec.BootstrapMode
	if bootstrapMode == "" {
		bootstrapMode = "configure" // Default to configure mode
	}

	// Reconcile based on deployment mode
	switch committerCoordinator.Spec.BootstrapMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, committerCoordinator); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateCommitterQueryServiceStatus(ctx, committerCoordinator, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, committerCoordinator); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateCommitterQueryServiceStatus(ctx, committerCoordinator, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid bootstrap mode: %s", committerCoordinator.Spec.BootstrapMode)
		log.Error(fmt.Errorf("%s", errorMsg), "Invalid bootstrap mode")
		r.updateCommitterQueryServiceStatus(ctx, committerCoordinator, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}

	// Update status to success
	r.updateCommitterQueryServiceStatus(ctx, committerCoordinator, fabricxv1alpha1.RunningStatus, "CommitterQueryService reconciled successfully")

	log.Info("CommitterQueryService reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only configuration resources)
func (r *CommitterQueryServiceReconciler) reconcileConfigureMode(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling CommitterQueryService in configure mode",
		"name", committerCoordinator.Name,
		"namespace", committerCoordinator.Namespace)

	// In configure mode, only create certificates
	if err := r.reconcileCertificates(ctx, committerCoordinator); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}
	log.Info("CommitterQueryService certificates created in configure mode")

	log.Info("CommitterQueryService configure mode reconciliation completed")
	return nil
}

// reconcileCertificates creates or updates certificates for the CommitterQueryService
func (r *CommitterQueryServiceReconciler) reconcileCertificates(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	log := logf.FromContext(ctx)

	// Check if enrollment is configured
	if committerCoordinator.Spec.Enrollment == nil {
		log.Info("No enrollment configuration found, skipping certificate creation")
		return nil
	}

	// Generate certificates for each type (each function handles its own existence check)
	var allCertificates []certs.ComponentCertificateData

	// Create sign certificate with component-specific SANS if available
	signCertConfig := &fabricxv1alpha1.CertificateConfig{
		CA: committerCoordinator.Spec.Enrollment.Sign.CA,
	}

	signRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    committerCoordinator.Name,
		ComponentType:    "coordinator",
		Namespace:        committerCoordinator.Namespace,
		OrdererGroupName: committerCoordinator.Name, // Using coordinator name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(committerCoordinator.Spec.MSPID, signCertConfig, "sign"),
		EnrollmentConfig: r.convertToEnrollmentConfig(committerCoordinator.Spec.MSPID, committerCoordinator.Spec.Enrollment),
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
		CA: committerCoordinator.Spec.Enrollment.TLS.CA,
	}
	// Use component-specific SANS if available, otherwise use enrollment SANS
	if committerCoordinator.Spec.SANS != nil {
		tlsCertConfig.SANS = committerCoordinator.Spec.SANS
	} else if committerCoordinator.Spec.Enrollment.TLS.SANS != nil {
		tlsCertConfig.SANS = committerCoordinator.Spec.Enrollment.TLS.SANS
	}

	tlsRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    committerCoordinator.Name,
		ComponentType:    "coordinator",
		Namespace:        committerCoordinator.Namespace,
		OrdererGroupName: committerCoordinator.Name, // Using coordinator name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(committerCoordinator.Spec.MSPID, tlsCertConfig, "tls"),
		EnrollmentConfig: r.convertToEnrollmentConfig(committerCoordinator.Spec.MSPID, committerCoordinator.Spec.Enrollment),
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
		if err := r.createCertificateSecrets(ctx, committerCoordinator, allCertificates); err != nil {
			return fmt.Errorf("failed to create certificate secrets: %w", err)
		}
	}

	log.Info("Certificates reconciled successfully", "coordinator", committerCoordinator.Name)
	return nil
}

// convertToCertConfig converts API certificate config to internal format
func (r *CommitterQueryServiceReconciler) convertToCertConfig(mspID string, apiConfig *fabricxv1alpha1.CertificateConfig, certType string) *certs.CertificateConfig {
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
func (r *CommitterQueryServiceReconciler) convertToEnrollmentConfig(mspID string, apiConfig *fabricxv1alpha1.EnrollmentConfig) *certs.EnrollmentConfig {
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
func (r *CommitterQueryServiceReconciler) createCertificateSecrets(
	ctx context.Context,
	committerCoordinator *fabricxv1alpha1.CommitterQueryService,
	certificates []certs.ComponentCertificateData,
) error {
	log := logf.FromContext(ctx)

	// Process each certificate in the slice
	for _, certData := range certificates {
		secretName := fmt.Sprintf("%s-%s-cert", committerCoordinator.Name, certData.CertType)

		// Check if secret already exists
		existingSecret := &corev1.Secret{}
		err := r.Get(ctx, client.ObjectKey{
			Name:      secretName,
			Namespace: committerCoordinator.Namespace,
		}, existingSecret)

		if err != nil {
			if errors.IsNotFound(err) {
				// Secret doesn't exist, create it
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: committerCoordinator.Namespace,
						Labels: map[string]string{
							"app":                      "fabric-x",
							"committerqueryservice":    committerCoordinator.Name,
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
				if err := controllerutil.SetControllerReference(committerCoordinator, secret, r.Scheme); err != nil {
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
				"committerqueryservice":    committerCoordinator.Name,
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
func (r *CommitterQueryServiceReconciler) reconcileDeployMode(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling CommitterQueryService in deploy mode",
		"name", committerCoordinator.Name,
		"namespace", committerCoordinator.Namespace,
		"bootstrapMode", committerCoordinator.Spec.BootstrapMode)

	// Check if bootstrap mode is set to deploy
	if committerCoordinator.Spec.BootstrapMode != "deploy" {
		log.Info("Bootstrap mode is not 'deploy', skipping deployment resources",
			"bootstrapMode", committerCoordinator.Spec.BootstrapMode)
		return nil
	}

	// 1. Create/Update certificates first
	if err := r.reconcileCertificates(ctx, committerCoordinator); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}

	// 2. Create/Update Secret for Coordinator configuration
	if err := r.reconcileSecret(ctx, committerCoordinator); err != nil {
		return fmt.Errorf("failed to reconcile secret: %w", err)
	}

	// 3. Create/Update Service for Coordinator
	if err := r.reconcileService(ctx, committerCoordinator); err != nil {
		return fmt.Errorf("failed to reconcile service: %w", err)
	}

	// 4. Create/Update Deployment for Coordinator
	if err := r.reconcileDeployment(ctx, committerCoordinator); err != nil {
		return fmt.Errorf("failed to reconcile deployment: %w", err)
	}

	// 5. Create/Update Ingress for Coordinator (if configured)
	if committerCoordinator.Spec.Ingress != nil {
		if err := r.reconcileIngress(ctx, committerCoordinator); err != nil {
			return fmt.Errorf("failed to reconcile ingress: %w", err)
		}
	}

	log.Info("CommitterQueryService deploy mode reconciliation completed")
	return nil
}

// reconcileConfigMap creates or updates the ConfigMap for Coordinator configuration
func (r *CommitterQueryServiceReconciler) reconcileSecret(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	log := logf.FromContext(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", committerCoordinator.Name),
			Namespace: committerCoordinator.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
	}

	// Prepare template data
	templateData := utils.CommitterQueryServiceTemplateData{
		Name:    committerCoordinator.Name,
		PartyID: committerCoordinator.Spec.PartyID,
		MSPID:   committerCoordinator.Spec.MSPID,
		Port:    9001,
	}

	// Add PostgreSQL configuration if provided
	if committerCoordinator.Spec.PostgreSQL != nil {
		templateData.PostgreSQL = &utils.PostgreSQLTemplateData{
			Host:           committerCoordinator.Spec.PostgreSQL.Host,
			Port:           committerCoordinator.Spec.PostgreSQL.Port,
			Database:       committerCoordinator.Spec.PostgreSQL.Database,
			Username:       committerCoordinator.Spec.PostgreSQL.Username,
			Password:       "", // Password will be fetched from secret
			MaxConnections: committerCoordinator.Spec.PostgreSQL.MaxConnections,
			MinConnections: committerCoordinator.Spec.PostgreSQL.MinConnections,
			LoadBalance:    committerCoordinator.Spec.PostgreSQL.LoadBalance,
			MaxElapsedTime: committerCoordinator.Spec.PostgreSQL.Retry.MaxElapsedTime,
		}

		// Fetch password from secret if provided
		if committerCoordinator.Spec.PostgreSQL.PasswordSecret != nil {
			secret := &corev1.Secret{}
			secretKey := client.ObjectKey{
				Name:      committerCoordinator.Spec.PostgreSQL.PasswordSecret.Name,
				Namespace: committerCoordinator.Spec.PostgreSQL.PasswordSecret.Namespace,
			}
			if err := r.Get(ctx, secretKey, secret); err != nil {
				return fmt.Errorf("failed to get PostgreSQL password secret: %w", err)
			}
			if password, ok := secret.Data[committerCoordinator.Spec.PostgreSQL.PasswordSecret.Key]; ok {
				templateData.PostgreSQL.Password = string(password)
			}
		}
	}

	// Build the query service configuration
	configContent := utils.BuildQueryServiceConfig(templateData)

	template := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", committerCoordinator.Name),
			Namespace: committerCoordinator.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"config.yaml": []byte(configContent),
		},
	}

	if err := r.updateSecret(ctx, committerCoordinator, secret, template); err != nil {
		log.Error(err, "Failed to update Secret", "name", secret.Name)
		return fmt.Errorf("failed to update Secret %s: %w", secret.Name, err)
	}

	log.Info("Secret reconciled successfully", "coordinator", committerCoordinator.Name)
	return nil
}

// updateSecret updates a secret with template data
func (r *CommitterQueryServiceReconciler) updateSecret(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService, secret *corev1.Secret, template *corev1.Secret) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(committerCoordinator, secret, r.Scheme); err != nil {
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

// reconcileService creates or updates the Service for Coordinator
func (r *CommitterQueryServiceReconciler) reconcileService(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", committerCoordinator.Name),
			Namespace: committerCoordinator.Namespace,
		},
	}

	template := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", committerCoordinator.Name),
			Namespace: committerCoordinator.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port:       9001,
					TargetPort: intstr.FromInt(9001),
					Protocol:   corev1.ProtocolTCP,
					Name:       "query-service",
				},
			},
			Selector: map[string]string{
				"app":     "query-service",
				"release": committerCoordinator.Name,
			},
		},
	}

	if err := r.updateService(ctx, committerCoordinator, service, template); err != nil {
		log.Error(err, "Failed to update Service", "name", service.Name)
		return fmt.Errorf("failed to update Service %s: %w", service.Name, err)
	}

	log.Info("Service reconciled successfully", "coordinator", committerCoordinator.Name)
	return nil
}

// updateService updates a service with template data
func (r *CommitterQueryServiceReconciler) updateService(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService, service *corev1.Service, template *corev1.Service) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(committerCoordinator, service, r.Scheme); err != nil {
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

// reconcileDeployment creates or updates the Deployment for Coordinator
func (r *CommitterQueryServiceReconciler) reconcileDeployment(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	log := logf.FromContext(ctx)

	// Compute combined hash of all mounted secrets/configmaps for rollout on change
	var hashParts []string

	// Hash config secret
	configSecretName := fmt.Sprintf("%s-config", committerCoordinator.Name)
	if hash, err := r.computeSecretHash(ctx, configSecretName, committerCoordinator.Namespace); err != nil {
		log.Error(err, "Failed to compute config secret hash", "secretName", configSecretName)
	} else {
		hashParts = append(hashParts, hash)
	}

	// Hash sign certificate secret
	signCertSecretName := fmt.Sprintf("%s-sign-cert", committerCoordinator.Name)
	if hash, err := r.computeSecretHash(ctx, signCertSecretName, committerCoordinator.Namespace); err != nil {
		log.V(1).Info("Sign cert secret not found or failed to hash", "secretName", signCertSecretName)
	} else {
		hashParts = append(hashParts, hash)
	}

	// Hash TLS certificate secret
	tlsCertSecretName := fmt.Sprintf("%s-tls-cert", committerCoordinator.Name)
	if hash, err := r.computeSecretHash(ctx, tlsCertSecretName, committerCoordinator.Namespace); err != nil {
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
			Name:      committerCoordinator.Name,
			Namespace: committerCoordinator.Namespace,
		},
	}

	template := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      committerCoordinator.Name,
			Namespace: committerCoordinator.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &committerCoordinator.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "query-service",
					"release": committerCoordinator.Name,
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "query-service",
						"release": committerCoordinator.Name,
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
							Name: "query-service",
							Image: fmt.Sprintf("%s:%s",
								func() string {
									if committerCoordinator.Spec.Image != "" {
										return committerCoordinator.Spec.Image
									}
									return "hyperledger/fabric-x-committer"
								}(),
								func() string {
									if committerCoordinator.Spec.ImageTag != "" {
										return committerCoordinator.Spec.ImageTag
									}
									return "0.1.5"
								}()),
							Command: []string{
								"committer",
							},
							Args: []string{
								"start-query",
								"--config=/config/config.yaml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "query-port",
									ContainerPort: 9001,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									ReadOnly:  true,
									MountPath: "/config",
								},
							},
							Resources: func() corev1.ResourceRequirements {
								if committerCoordinator.Spec.Resources != nil {
									return *committerCoordinator.Spec.Resources
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
									SecretName: fmt.Sprintf("%s-config", committerCoordinator.Name),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := r.updateDeployment(ctx, committerCoordinator, deployment, template); err != nil {
		log.Error(err, "Failed to update Deployment", "name", deployment.Name)
		return fmt.Errorf("failed to update Deployment %s: %w", deployment.Name, err)
	}

	log.Info("Deployment reconciled successfully", "coordinator", committerCoordinator.Name)
	return nil
}

// updateDeployment updates a deployment with template data
func (r *CommitterQueryServiceReconciler) updateDeployment(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService, deployment *appsv1.Deployment, template *appsv1.Deployment) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(committerCoordinator, deployment, r.Scheme); err != nil {
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
func (r *CommitterQueryServiceReconciler) computeSecretHash(ctx context.Context, secretName, namespace string) (string, error) {
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

// reconcileIngress creates or updates the Ingress for Coordinator
func (r *CommitterQueryServiceReconciler) reconcileIngress(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	// TODO: Implement Ingress reconciliation
	// This would create/update an Ingress resource based on the ingress configuration
	return nil
}

// handleDeletion handles the deletion of a CommitterQueryService
func (r *CommitterQueryServiceReconciler) handleDeletion(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Handling CommitterQueryService deletion",
		"name", committerCoordinator.Name,
		"namespace", committerCoordinator.Namespace)

	// Set status to indicate deletion
	r.updateCommitterQueryServiceStatus(ctx, committerCoordinator, fabricxv1alpha1.PendingStatus, "Deleting CommitterQueryService resources")

	// TODO: Clean up resources based on deployment mode
	// - Delete Deployments/StatefulSets
	// - Delete Services
	// - Delete PVCs
	// - Delete ConfigMaps and Secrets

	// Remove finalizer
	if err := r.removeFinalizer(ctx, committerCoordinator); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateCommitterQueryServiceStatus(ctx, committerCoordinator, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("CommitterQueryService deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the CommitterQueryService
func (r *CommitterQueryServiceReconciler) ensureFinalizer(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	if !utils.ContainsString(committerCoordinator.Finalizers, CommitterQueryServiceFinalizerName) {
		committerCoordinator.Finalizers = append(committerCoordinator.Finalizers, CommitterQueryServiceFinalizerName)
		return r.Update(ctx, committerCoordinator)
	}
	return nil
}

// removeFinalizer removes the finalizer from the CommitterQueryService
func (r *CommitterQueryServiceReconciler) removeFinalizer(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService) error {
	committerCoordinator.Finalizers = utils.RemoveString(committerCoordinator.Finalizers, CommitterQueryServiceFinalizerName)
	return r.Update(ctx, committerCoordinator)
}

// updateCommitterQueryServiceStatus updates the CommitterQueryService status with the given status and message
func (r *CommitterQueryServiceReconciler) updateCommitterQueryServiceStatus(ctx context.Context, committerCoordinator *fabricxv1alpha1.CommitterQueryService, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating CommitterQueryService status",
		"name", committerCoordinator.Name,
		"namespace", committerCoordinator.Namespace,
		"status", status,
		"message", message)

	// Update the status
	committerCoordinator.Status.Status = status
	committerCoordinator.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	committerCoordinator.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, committerCoordinator); err != nil {
		log.Error(err, "Failed to update CommitterQueryService status")
	} else {
		log.Info("CommitterQueryService status updated successfully",
			"name", committerCoordinator.Name,
			"namespace", committerCoordinator.Namespace,
			"status", status,
			"message", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CommitterQueryServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.CommitterQueryService{}).
		Named("committerqueryservice").
		Complete(r)
}
