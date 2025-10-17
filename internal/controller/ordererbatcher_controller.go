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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	// Istio imports
	istioapinetworkingv1alpha3 "istio.io/api/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

const (
	// OrdererBatcherFinalizerName is the name of the finalizer used by OrdererBatcher
	OrdererBatcherFinalizerName = "ordererbatcher.fabricx.kfsoft.tech/finalizer"

	// Service and deployment constants
	BatcherServicePort = 7151
	BatcherTargetPort  = 7151
)

// Helper functions for consistent naming and port configuration

// getServiceName returns the service name for the batcher
func (r *OrdererBatcherReconciler) getServiceName(ordererBatcher *fabricxv1alpha1.OrdererBatcher) string {
	return utils.GetServiceName(ordererBatcher.Name)
}

// getDeploymentName returns the deployment name for the batcher
func (r *OrdererBatcherReconciler) getDeploymentName(ordererBatcher *fabricxv1alpha1.OrdererBatcher) string {
	return ordererBatcher.Name
}

// getServicePort returns the service port for the batcher
func (r *OrdererBatcherReconciler) getServicePort() int32 {
	return BatcherServicePort
}

// getTargetPort returns the target port for the batcher
func (r *OrdererBatcherReconciler) getTargetPort() int {
	return BatcherTargetPort
}

// getServiceFQDN returns the fully qualified domain name for the service
func (r *OrdererBatcherReconciler) getServiceFQDN(ordererBatcher *fabricxv1alpha1.OrdererBatcher) string {
	return utils.GetServiceFQDN(ordererBatcher.Name, ordererBatcher.Namespace)
}

// computeConfigMapHash computes a hash of the ConfigMap data to trigger deployment updates
func (r *OrdererBatcherReconciler) computeConfigMapHash(ctx context.Context, configMapName, namespace string) (string, error) {
	configMap := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: namespace,
	}, configMap)
	if err != nil {
		return "", fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	// Create a deterministic string representation of the ConfigMap data
	var dataStrings []string
	for key, value := range configMap.Data {
		dataStrings = append(dataStrings, fmt.Sprintf("%s=%s", key, value))
	}
	// Sort to ensure deterministic ordering
	sort.Strings(dataStrings)

	// Concatenate all data
	dataString := strings.Join(dataStrings, "|")

	// Compute SHA256 hash
	hash := sha256.Sum256([]byte(dataString))
	return hex.EncodeToString(hash[:]), nil
}

// OrdererBatcherReconciler reconciles a OrdererBatcher object
type OrdererBatcherReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererbatchers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererbatchers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=ordererbatchers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OrdererBatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererBatcher reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the OrdererBatcher status to failed
			ordererBatcher := &fabricxv1alpha1.OrdererBatcher{}
			if err := r.Get(ctx, req.NamespacedName, ordererBatcher); err == nil {
				panicMsg := fmt.Sprintf("Panic in OrdererBatcher reconciliation: %v", panicErr)
				r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var ordererBatcher fabricxv1alpha1.OrdererBatcher
	if err := r.Get(ctx, req.NamespacedName, &ordererBatcher); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the OrdererBatcher is being deleted
	if !ordererBatcher.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &ordererBatcher)
	}

	// Set initial status if not set
	if ordererBatcher.Status.Status == "" {
		r.updateOrdererBatcherStatus(ctx, &ordererBatcher, fabricxv1alpha1.PendingStatus, "Initializing OrdererBatcher")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &ordererBatcher); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateOrdererBatcherStatus(ctx, &ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the OrdererBatcher
	if err := r.reconcileOrdererBatcher(ctx, &ordererBatcher); err != nil {
		// The reconcileOrdererBatcher method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if ordererBatcher.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile OrdererBatcher: %v", err)
			r.updateOrdererBatcherStatus(ctx, &ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile OrdererBatcher")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute to ensure continuous monitoring
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileOrdererBatcher handles the reconciliation of an OrdererBatcher
func (r *OrdererBatcherReconciler) reconcileOrdererBatcher(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererBatcher reconciliation",
				"ordererBatcher", ordererBatcher.Name, "namespace", ordererBatcher.Namespace)

			// Update the OrdererBatcher status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererBatcher reconciliation: %v", panicErr)
			r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Starting OrdererBatcher reconciliation",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace,
		"bootstrapMode", ordererBatcher.Spec.BootstrapMode,
		"shardID", ordererBatcher.Spec.ShardID)

	// Check bootstrap mode - only deploy when bootstrapMode is "deploy"
	bootstrapMode := ordererBatcher.Spec.BootstrapMode
	if bootstrapMode == "" {
		bootstrapMode = "configure" // Default to configure mode
	}

	// Reconcile based on deployment mode
	switch ordererBatcher.Spec.BootstrapMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, ordererBatcher); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, ordererBatcher); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid bootstrap mode: %s", ordererBatcher.Spec.BootstrapMode)
		log.Error(fmt.Errorf("%s", errorMsg), "Invalid bootstrap mode")
		r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}

	// Update status to success
	r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.RunningStatus, "OrdererBatcher reconciled successfully")

	log.Info("OrdererBatcher reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only configuration resources)
func (r *OrdererBatcherReconciler) reconcileConfigureMode(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererBatcher in configure mode",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace)

	// In configure mode, create certificates and verify genesis block
	if err := r.reconcileCertificates(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}
	log.Info("OrdererBatcher certificates created in configure mode")

	// Verify genesis block secret exists
	if err := r.reconcileGenesisBlock(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile genesis block: %w", err)
	}
	log.Info("OrdererBatcher genesis block verified in configure mode")

	log.Info("OrdererBatcher configure mode reconciliation completed")
	return nil
}

// reconcileCertificates creates or updates certificates for the OrdererBatcher
func (r *OrdererBatcherReconciler) reconcileCertificates(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	// Check if enrollment is configured
	if ordererBatcher.Spec.Enrollment == nil {
		log.Info("No enrollment configuration found, skipping certificate creation")
		return nil
	}

	// Generate certificates for each type (each function handles its own existence check)
	var allCertificates []certs.ComponentCertificateData

	// Create sign certificate with component-specific SANS if available
	signCertConfig := &fabricxv1alpha1.CertificateConfig{
		CA: ordererBatcher.Spec.Enrollment.Sign.CA,
	}

	signRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    ordererBatcher.Name,
		ComponentType:    "batcher",
		Namespace:        ordererBatcher.Namespace,
		OrdererGroupName: ordererBatcher.Name, // Using batcher name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(ordererBatcher.Spec.MSPID, signCertConfig, "sign"),
		EnrollmentConfig: r.convertToEnrollmentConfig(ordererBatcher.Spec.MSPID, ordererBatcher.Spec.Enrollment),
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
		CA: ordererBatcher.Spec.Enrollment.TLS.CA,
	}
	// Use component-specific SANS if available, otherwise use enrollment SANS
	if ordererBatcher.Spec.SANS != nil {
		tlsCertConfig.SANS = ordererBatcher.Spec.SANS
	} else if ordererBatcher.Spec.Enrollment.TLS.SANS != nil {
		tlsCertConfig.SANS = ordererBatcher.Spec.Enrollment.TLS.SANS
	}

	tlsRequest := certs.OrdererGroupCertificateRequest{
		ComponentName:    ordererBatcher.Name,
		ComponentType:    "batcher",
		Namespace:        ordererBatcher.Namespace,
		OrdererGroupName: ordererBatcher.Name, // Using batcher name as orderer group name for individual instances
		CertConfig:       r.convertToCertConfig(ordererBatcher.Spec.MSPID, tlsCertConfig, "tls"),
		EnrollmentConfig: r.convertToEnrollmentConfig(ordererBatcher.Spec.MSPID, ordererBatcher.Spec.Enrollment),
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
		if err := r.createCertificateSecrets(ctx, ordererBatcher, allCertificates); err != nil {
			return fmt.Errorf("failed to create certificate secrets: %w", err)
		}
	}

	log.Info("Certificates reconciled successfully", "batcher", ordererBatcher.Name)
	return nil
}

// convertToCertConfig converts API certificate config to internal format
func (r *OrdererBatcherReconciler) convertToCertConfig(mspID string, apiConfig *fabricxv1alpha1.CertificateConfig, certType string) *certs.CertificateConfig {
	if apiConfig == nil {
		return nil
	}

	config := &certs.CertificateConfig{
		MSPID: mspID,
	}

	// Add CA configuration if provided
	if apiConfig.CA != nil {
		config.CA = &certs.CACertificateConfig{
			CAName:       apiConfig.CA.CAName,
			CAHost:       apiConfig.CA.CAHost,
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
func (r *OrdererBatcherReconciler) convertToEnrollmentConfig(mspID string, apiConfig *fabricxv1alpha1.EnrollmentConfig) *certs.EnrollmentConfig {
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
func (r *OrdererBatcherReconciler) createCertificateSecrets(
	ctx context.Context,
	ordererBatcher *fabricxv1alpha1.OrdererBatcher,
	certificates []certs.ComponentCertificateData,
) error {
	log := logf.FromContext(ctx)

	// Process each certificate in the slice
	for _, certData := range certificates {
		secretName := fmt.Sprintf("%s-%s-cert", ordererBatcher.Name, certData.CertType)

		// Check if secret already exists
		existingSecret := &corev1.Secret{}
		err := r.Client.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: ordererBatcher.Namespace,
		}, existingSecret)

		if err != nil {
			if errors.IsNotFound(err) {
				// Secret doesn't exist, create it
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: ordererBatcher.Namespace,
						Labels: map[string]string{
							"app":                      "fabric-x",
							"ordererbatcher":           ordererBatcher.Name,
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
				if err := controllerutil.SetControllerReference(ordererBatcher, secret, r.Scheme); err != nil {
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
				"ordererbatcher":           ordererBatcher.Name,
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

// reconcileConfigMap creates or updates the ConfigMap for Batcher configuration
func (r *OrdererBatcherReconciler) reconcileConfigMap(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", ordererBatcher.Name),
			Namespace: ordererBatcher.Namespace,
		},
	}

	// Prepare template data
	templateData := utils.BatcherTemplateData{
		Name:    ordererBatcher.Name,
		PartyID: ordererBatcher.Spec.PartyID,
		MSPID:   ordererBatcher.Spec.MSPID,
		ShardID: ordererBatcher.Spec.ShardID,
		Port:    7151,
	}

	// Execute the template using the shared utility
	configContent, err := utils.ExecuteTemplateWithValidation(utils.BatcherConfigTemplate, templateData)
	if err != nil {
		return fmt.Errorf("failed to execute batcher config template: %w", err)
	}

	// MSP config.yaml content
	mspConfigContent := `NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/ca.pem
    OrganizationalUnitIdentifier: orderer
`

	template := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", ordererBatcher.Name),
			Namespace: ordererBatcher.Namespace,
		},
		Data: map[string]string{
			"node_config.yaml": configContent,
			"msp_config.yaml":  mspConfigContent,
		},
	}

	if err := r.updateConfigMap(ctx, ordererBatcher, configMap, template); err != nil {
		log.Error(err, "Failed to update ConfigMap", "name", configMap.Name)
		return fmt.Errorf("failed to update ConfigMap %s: %w", configMap.Name, err)
	}

	log.Info("ConfigMap reconciled successfully", "batcher", ordererBatcher.Name)
	return nil
}

// reconcileService creates or updates the Service for Batcher
func (r *OrdererBatcherReconciler) reconcileService(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ordererBatcher.Name,
			Namespace: ordererBatcher.Namespace,
		},
	}
	template := r.getServiceTemplate(ordererBatcher)
	if err := r.updateService(ctx, ordererBatcher, service, template); err != nil {
		log.Error(err, "Failed to update Service", "name", service.Name)
		return fmt.Errorf("failed to update Service %s: %w", service.Name, err)
	}

	log.Info("Service reconciled successfully", "batcher", ordererBatcher.Name)
	return nil
}

// reconcileDeployment creates or updates the Deployment for Batcher
func (r *OrdererBatcherReconciler) reconcileDeployment(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ordererBatcher.Name,
			Namespace: ordererBatcher.Namespace,
		},
	}
	template := r.getDeploymentTemplate(ctx, ordererBatcher)
	if err := r.updateDeployment(ctx, ordererBatcher, deployment, template); err != nil {
		log.Error(err, "Failed to update Deployment", "name", deployment.Name)
		return fmt.Errorf("failed to update Deployment %s: %w", deployment.Name, err)
	}

	log.Info("Deployment reconciled successfully", "batcher", ordererBatcher.Name)
	return nil
}

// reconcilePVC creates or updates the PVC for Batcher
func (r *OrdererBatcherReconciler) reconcilePVC(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-store-pvc", ordererBatcher.Name),
			Namespace: ordererBatcher.Namespace,
		},
	}

	// Check if PVC exists
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      pvc.Name,
		Namespace: pvc.Namespace,
	}, pvc)

	if err != nil {
		if errors.IsNotFound(err) {
			// PVC doesn't exist, create it
			storageClassName := ""
			if ordererBatcher.Spec.StorageClassName != "" {
				storageClassName = ordererBatcher.Spec.StorageClassName
			}

			accessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
			if len(ordererBatcher.Spec.PVCAccessModes) > 0 {
				accessModes = []corev1.PersistentVolumeAccessMode{}
				for _, mode := range ordererBatcher.Spec.PVCAccessModes {
					switch mode {
					case "ReadWriteOnce":
						accessModes = append(accessModes, corev1.ReadWriteOnce)
					case "ReadOnlyMany":
						accessModes = append(accessModes, corev1.ReadOnlyMany)
					case "ReadWriteMany":
						accessModes = append(accessModes, corev1.ReadWriteMany)
					}
				}
			}

			storageSize := "10Gi"
			if ordererBatcher.Spec.PVCStorageSize != "" {
				storageSize = ordererBatcher.Spec.PVCStorageSize
			}

			pvc.Spec = corev1.PersistentVolumeClaimSpec{
				AccessModes: accessModes,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(storageSize),
					},
				},
				StorageClassName: func() *string {
					if storageClassName != "" {
						return &storageClassName
					}
					return nil
				}(),
			}

			// Set controller reference
			if err := controllerutil.SetControllerReference(ordererBatcher, pvc, r.Scheme); err != nil {
				return fmt.Errorf("failed to set controller reference for PVC: %w", err)
			}

			// Use Create with retry logic for conflict resolution
			if err := r.createPVCWithRetry(ctx, pvc); err != nil {
				return fmt.Errorf("failed to create PVC: %w", err)
			}

			log.Info("Created PVC", "name", pvc.Name, "namespace", pvc.Namespace)
		} else {
			return fmt.Errorf("failed to get PVC: %w", err)
		}
	} else {
		// PVC exists, check if it needs to be updated
		needsUpdate := false
		updatedPVC := pvc.DeepCopy()

		// Check storage class
		if ordererBatcher.Spec.StorageClassName != "" {
			expectedStorageClass := ordererBatcher.Spec.StorageClassName
			if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != expectedStorageClass {
				updatedPVC.Spec.StorageClassName = &expectedStorageClass
				needsUpdate = true
			}
		}

		// Check access modes
		expectedAccessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		if len(ordererBatcher.Spec.PVCAccessModes) > 0 {
			expectedAccessModes = []corev1.PersistentVolumeAccessMode{}
			for _, mode := range ordererBatcher.Spec.PVCAccessModes {
				switch mode {
				case "ReadWriteOnce":
					expectedAccessModes = append(expectedAccessModes, corev1.ReadWriteOnce)
				case "ReadOnlyMany":
					expectedAccessModes = append(expectedAccessModes, corev1.ReadOnlyMany)
				case "ReadWriteMany":
					expectedAccessModes = append(expectedAccessModes, corev1.ReadWriteMany)
				}
			}
		}

		if !reflect.DeepEqual(pvc.Spec.AccessModes, expectedAccessModes) {
			updatedPVC.Spec.AccessModes = expectedAccessModes
			needsUpdate = true
		}

		// Check storage size
		expectedStorageSize := "10Gi"
		if ordererBatcher.Spec.PVCStorageSize != "" {
			expectedStorageSize = ordererBatcher.Spec.PVCStorageSize
		}

		currentStorageSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		expectedStorageQuantity := resource.MustParse(expectedStorageSize)
		if !currentStorageSize.Equal(expectedStorageQuantity) {
			updatedPVC.Spec.Resources.Requests[corev1.ResourceStorage] = expectedStorageQuantity
			needsUpdate = true
		}

		if needsUpdate {
			// Use Update with retry logic for conflict resolution
			if err := r.updatePVCWithRetry(ctx, updatedPVC); err != nil {
				return fmt.Errorf("failed to update PVC: %w", err)
			}
			log.Info("Updated PVC", "name", pvc.Name, "namespace", pvc.Namespace)
		} else {
			log.Info("PVC already exists and is up to date", "name", pvc.Name, "namespace", pvc.Namespace)
		}
	}

	return nil
}

// createPVCWithRetry creates a PVC with retry logic for conflict resolution
func (r *OrdererBatcherReconciler) createPVCWithRetry(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		err := r.Client.Create(ctx, pvc)
		if err == nil {
			return nil
		}

		// If it's a conflict error, retry
		if errors.IsConflict(err) {
			log := logf.FromContext(ctx)
			log.Info("PVC creation conflict, retrying", "name", pvc.Name, "attempt", i+1)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		// For other errors, return immediately
		return err
	}

	return fmt.Errorf("failed to create PVC after %d retries", maxRetries)
}

// updatePVCWithRetry updates a PVC with retry logic for conflict resolution
func (r *OrdererBatcherReconciler) updatePVCWithRetry(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		err := r.Client.Update(ctx, pvc)
		if err == nil {
			return nil
		}

		// If it's a conflict error, get the latest version and retry
		if errors.IsConflict(err) {
			log := logf.FromContext(ctx)
			log.Info("PVC update conflict, getting latest version and retrying", "name", pvc.Name, "attempt", i+1)

			// Get the latest version
			latestPVC := &corev1.PersistentVolumeClaim{}
			if err := r.Client.Get(ctx, types.NamespacedName{
				Name:      pvc.Name,
				Namespace: pvc.Namespace,
			}, latestPVC); err != nil {
				return fmt.Errorf("failed to get latest PVC version: %w", err)
			}

			// Update the resource version
			pvc.ResourceVersion = latestPVC.ResourceVersion
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}

		// For other errors, return immediately
		return err
	}

	return fmt.Errorf("failed to update PVC after %d retries", maxRetries)
}

// reconcileGenesisBlock creates or updates the genesis block secret for the OrdererBatcher
func (r *OrdererBatcherReconciler) reconcileGenesisBlock(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	// Check if genesis configuration is provided
	if ordererBatcher.Spec.Genesis.SecretName == "" {
		log.Info("No genesis block configuration found, skipping genesis block reconciliation")
		return nil
	}

	// Verify that the genesis block secret exists
	genesisSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: func() string {
			if ordererBatcher.Spec.Genesis.SecretNamespace != "" {
				return ordererBatcher.Spec.Genesis.SecretNamespace
			}
			return ordererBatcher.Namespace
		}(),
		Name: ordererBatcher.Spec.Genesis.SecretName,
	}, genesisSecret)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "Genesis block secret not found",
				"secretName", ordererBatcher.Spec.Genesis.SecretName,
				"secretNamespace", func() string {
					if ordererBatcher.Spec.Genesis.SecretNamespace != "" {
						return ordererBatcher.Spec.Genesis.SecretNamespace
					}
					return ordererBatcher.Namespace
				}())
			return fmt.Errorf("genesis block secret not found: %w", err)
		}
		return fmt.Errorf("failed to get genesis block secret: %w", err)
	}

	// Check if the genesis block data exists in the secret
	genesisKey := ordererBatcher.Spec.Genesis.SecretKey
	if genesisKey == "" {
		genesisKey = "genesis.block" // Default key name
	}

	if _, exists := genesisSecret.Data[genesisKey]; !exists {
		log.Error(fmt.Errorf("genesis block data not found in secret"),
			"Genesis block data not found in secret",
			"secretName", ordererBatcher.Spec.Genesis.SecretName,
			"secretKey", genesisKey)
		return fmt.Errorf("genesis block data not found in secret %s with key %s", ordererBatcher.Spec.Genesis.SecretName, genesisKey)
	}

	log.Info("Genesis block secret verified successfully",
		"secretName", ordererBatcher.Spec.Genesis.SecretName,
		"secretKey", genesisKey)
	return nil
}

// reconcileIngress creates or updates the Ingress for Batcher
func (r *OrdererBatcherReconciler) reconcileIngress(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	// TODO: Implement Ingress reconciliation
	// This would create/update an Ingress resource based on the ingress configuration
	return nil
}

// reconcileIstioGateway creates or updates the Istio Gateway for Batcher
func (r *OrdererBatcherReconciler) reconcileIstioGateway(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	// Check if Istio configuration is provided
	if ordererBatcher.Spec.Ingress == nil || ordererBatcher.Spec.Ingress.Istio == nil {
		log.Info("No Istio configuration found, skipping Gateway creation")
		return nil
	}

	istioConfig := ordererBatcher.Spec.Ingress.Istio
	gatewayName := fmt.Sprintf("%s-gateway", ordererBatcher.Name)

	// Create Gateway resource template
	gatewayTemplate := &istionetworkingv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: ordererBatcher.Namespace,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererbatcher":           ordererBatcher.Name,
				"fabricx.kfsoft.tech/type": "gateway",
			},
		},
		Spec: istioapinetworkingv1alpha3.Gateway{
			Selector: map[string]string{
				"istio": istioConfig.IngressGateway,
			},
			Servers: []*istioapinetworkingv1alpha3.Server{
				{
					Port: &istioapinetworkingv1alpha3.Port{
						Number:   uint32(istioConfig.Port),
						Name:     "tls",
						Protocol: "TLS",
					},
					Hosts: istioConfig.Hosts,
					Tls: &istioapinetworkingv1alpha3.ServerTLSSettings{
						Mode: istioapinetworkingv1alpha3.ServerTLSSettings_PASSTHROUGH,
					},
				},
			},
		},
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(ordererBatcher, gatewayTemplate, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for Gateway: %w", err)
	}

	// Check if Gateway already exists
	existingGateway := &istionetworkingv1beta1.Gateway{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      gatewayName,
		Namespace: ordererBatcher.Namespace,
	}, existingGateway)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new Gateway
			if err := r.Client.Create(ctx, gatewayTemplate); err != nil {
				return fmt.Errorf("failed to create Gateway: %w", err)
			}
			log.Info("Created Istio Gateway", "gateway", gatewayName)
		} else {
			return fmt.Errorf("failed to get existing Gateway: %w", err)
		}
	} else {
		// Update existing Gateway - always update to ensure it's current
		existingGateway.Spec = gatewayTemplate.Spec
		existingGateway.Labels = gatewayTemplate.Labels
		if err := r.Client.Update(ctx, existingGateway); err != nil {
			return fmt.Errorf("failed to update Gateway: %w", err)
		}
		log.Info("Updated Istio Gateway", "gateway", gatewayName)
	}

	log.Info("Istio Gateway reconciled successfully", "gateway", gatewayName)
	return nil
}

// reconcileIstioVirtualService creates or updates the Istio VirtualService for Batcher
func (r *OrdererBatcherReconciler) reconcileIstioVirtualService(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	// Check if Istio configuration is provided
	if ordererBatcher.Spec.Ingress == nil || ordererBatcher.Spec.Ingress.Istio == nil {
		log.Info("No Istio configuration found, skipping VirtualService creation")
		return nil
	}

	istioConfig := ordererBatcher.Spec.Ingress.Istio
	virtualServiceName := fmt.Sprintf("%s-virtualservice", ordererBatcher.Name)
	gatewayName := fmt.Sprintf("%s-gateway", ordererBatcher.Name)

	// Create VirtualService resource template
	virtualServiceTemplate := &istionetworkingv1beta1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtualServiceName,
			Namespace: ordererBatcher.Namespace,
			Labels: map[string]string{
				"app":                      "fabric-x",
				"ordererbatcher":           ordererBatcher.Name,
				"fabricx.kfsoft.tech/type": "virtualservice",
			},
		},
		Spec: istioapinetworkingv1alpha3.VirtualService{
			Hosts:    istioConfig.Hosts,
			Gateways: []string{gatewayName},
			Tls: []*istioapinetworkingv1alpha3.TLSRoute{
				{
					Match: []*istioapinetworkingv1alpha3.TLSMatchAttributes{
						{
							Port:     uint32(istioConfig.Port),
							SniHosts: istioConfig.Hosts,
						},
					},
					Route: []*istioapinetworkingv1alpha3.RouteDestination{
						{
							Destination: &istioapinetworkingv1alpha3.Destination{
								Host: fmt.Sprintf("%s.%s.svc.cluster.local", ordererBatcher.Name, ordererBatcher.Namespace),
								Port: &istioapinetworkingv1alpha3.PortSelector{
									Number: 7151, // Batcher port
								},
							},
							Weight: 100,
						},
					},
				},
			},
		},
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(ordererBatcher, virtualServiceTemplate, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for VirtualService: %w", err)
	}

	// Check if VirtualService already exists
	existingVirtualService := &istionetworkingv1beta1.VirtualService{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      virtualServiceName,
		Namespace: ordererBatcher.Namespace,
	}, existingVirtualService)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new VirtualService
			if err := r.Client.Create(ctx, virtualServiceTemplate); err != nil {
				return fmt.Errorf("failed to create VirtualService: %w", err)
			}
			log.Info("Created Istio VirtualService", "virtualService", virtualServiceName)
		} else {
			return fmt.Errorf("failed to get existing VirtualService: %w", err)
		}
	} else {
		// Update existing VirtualService - always update to ensure it's current
		existingVirtualService.Spec = virtualServiceTemplate.Spec
		existingVirtualService.Labels = virtualServiceTemplate.Labels
		if err := r.Client.Update(ctx, existingVirtualService); err != nil {
			return fmt.Errorf("failed to update VirtualService: %w", err)
		}
		log.Info("Updated Istio VirtualService", "virtualService", virtualServiceName)
	}

	log.Info("Istio VirtualService reconciled successfully", "virtualService", virtualServiceName)
	return nil
}

// reconcileIstioResources creates or updates Istio Gateway and VirtualService resources
func (r *OrdererBatcherReconciler) reconcileIstioResources(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	// Check if Istio configuration is provided
	if ordererBatcher.Spec.Ingress == nil || ordererBatcher.Spec.Ingress.Istio == nil {
		log.Info("No Istio configuration found, skipping Istio resources")
		return nil
	}

	// Reconcile Gateway
	if err := r.reconcileIstioGateway(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile Istio Gateway: %w", err)
	}

	// Reconcile VirtualService
	if err := r.reconcileIstioVirtualService(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile Istio VirtualService: %w", err)
	}

	log.Info("Istio resources reconciled successfully")
	return nil
}

// cleanupIstioResources cleans up Istio Gateway and VirtualService resources
func (r *OrdererBatcherReconciler) cleanupIstioResources(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	// Check if Istio configuration is provided
	if ordererBatcher.Spec.Ingress == nil || ordererBatcher.Spec.Ingress.Istio == nil {
		log.Info("No Istio configuration found, skipping Istio resources cleanup")
		return nil
	}

	gatewayName := fmt.Sprintf("%s-gateway", ordererBatcher.Name)
	virtualServiceName := fmt.Sprintf("%s-virtualservice", ordererBatcher.Name)

	// Delete Gateway
	gateway := &istionetworkingv1beta1.Gateway{}
	gateway.SetName(gatewayName)
	gateway.SetNamespace(ordererBatcher.Namespace)

	if err := r.Client.Delete(ctx, gateway); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete Istio Gateway", "name", gatewayName)
	} else {
		log.Info("Deleted Istio Gateway", "name", gatewayName)
	}

	// Delete VirtualService
	virtualService := &istionetworkingv1beta1.VirtualService{}
	virtualService.SetName(virtualServiceName)
	virtualService.SetNamespace(ordererBatcher.Namespace)

	if err := r.Client.Delete(ctx, virtualService); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete Istio VirtualService", "name", virtualServiceName)
	} else {
		log.Info("Deleted Istio VirtualService", "name", virtualServiceName)
	}

	log.Info("Istio resources cleanup completed")
	return nil
}

// reconcileDeployMode handles reconciliation in deploy mode (full deployment)
func (r *OrdererBatcherReconciler) reconcileDeployMode(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling OrdererBatcher in deploy mode",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace,
		"shardID", ordererBatcher.Spec.ShardID)

	// 1. Create/Update certificates first
	if err := r.reconcileCertificates(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}

	// 2. Create/Update genesis block secret
	if err := r.reconcileGenesisBlock(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile genesis block: %w", err)
	}

	// 3. Create/Update ConfigMap for Batcher configuration
	if err := r.reconcileConfigMap(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile configmap: %w", err)
	}

	// 4. Create/Update Service for Batcher
	if err := r.reconcileService(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile service: %w", err)
	}

	// 5. Create/Update Deployment for Batcher
	if err := r.reconcileDeployment(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile deployment: %w", err)
	}

	// 6. Create/Update PVC for Batcher
	if err := r.reconcilePVC(ctx, ordererBatcher); err != nil {
		return fmt.Errorf("failed to reconcile PVC: %w", err)
	}

	// 7. Create/Update Ingress for Batcher (if configured)
	if ordererBatcher.Spec.Ingress != nil {
		if err := r.reconcileIngress(ctx, ordererBatcher); err != nil {
			return fmt.Errorf("failed to reconcile ingress: %w", err)
		}
	}

	// 8. Create/Update Istio Gateway and VirtualService (if Istio is configured)
	if ordererBatcher.Spec.Ingress != nil && ordererBatcher.Spec.Ingress.Istio != nil {
		if err := r.reconcileIstioResources(ctx, ordererBatcher); err != nil {
			return fmt.Errorf("failed to reconcile Istio resources: %w", err)
		}
	}

	log.Info("OrdererBatcher deploy mode reconciliation completed")
	return nil
}

// handleDeletion handles the deletion of an OrdererBatcher
func (r *OrdererBatcherReconciler) handleDeletion(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in OrdererBatcher deletion",
				"ordererBatcher", ordererBatcher.Name, "namespace", ordererBatcher.Namespace)

			// Update the OrdererBatcher status to failed
			panicMsg := fmt.Sprintf("Panic in OrdererBatcher deletion: %v", panicErr)
			r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, panicMsg)
		}
	}()

	log.Info("Handling OrdererBatcher deletion",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace)

	// Set status to indicate deletion
	r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.PendingStatus, "Deleting OrdererBatcher resources")

	// Clean up Istio resources if they exist
	if ordererBatcher.Spec.Ingress != nil && ordererBatcher.Spec.Ingress.Istio != nil {
		if err := r.cleanupIstioResources(ctx, ordererBatcher); err != nil {
			log.Error(err, "Failed to cleanup Istio resources")
		}
	}

	// TODO: Clean up resources based on deployment mode
	// - Delete Deployments/StatefulSets
	// - Delete Services
	// - Delete PVCs
	// - Delete ConfigMaps and Secrets

	// Remove finalizer
	if err := r.removeFinalizer(ctx, ordererBatcher); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateOrdererBatcherStatus(ctx, ordererBatcher, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("OrdererBatcher deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the OrdererBatcher
func (r *OrdererBatcherReconciler) ensureFinalizer(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	if !utils.ContainsString(ordererBatcher.Finalizers, OrdererBatcherFinalizerName) {
		ordererBatcher.Finalizers = append(ordererBatcher.Finalizers, OrdererBatcherFinalizerName)
		return r.Update(ctx, ordererBatcher)
	}
	return nil
}

// removeFinalizer removes the finalizer from the OrdererBatcher
func (r *OrdererBatcherReconciler) removeFinalizer(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) error {
	ordererBatcher.Finalizers = utils.RemoveString(ordererBatcher.Finalizers, OrdererBatcherFinalizerName)
	return r.Update(ctx, ordererBatcher)
}

// updateOrdererBatcherStatus updates the OrdererBatcher status with the given status and message
func (r *OrdererBatcherReconciler) updateOrdererBatcherStatus(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating OrdererBatcher status",
		"name", ordererBatcher.Name,
		"namespace", ordererBatcher.Namespace,
		"status", status,
		"message", message)

	// Update the status
	ordererBatcher.Status.Status = status
	ordererBatcher.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	ordererBatcher.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, ordererBatcher); err != nil {
		log.Error(err, "Failed to update OrdererBatcher status")
	} else {
		log.Info("OrdererBatcher status updated successfully",
			"name", ordererBatcher.Name,
			"namespace", ordererBatcher.Namespace,
			"status", status,
			"message", message)
	}
}

// updateConfigMap updates a configmap with template data
func (r *OrdererBatcherReconciler) updateConfigMap(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher, configMap *corev1.ConfigMap, template *corev1.ConfigMap) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ordererBatcher, configMap, r.Scheme); err != nil {
			return err
		}

		// Update configmap data
		configMap.Data = template.Data

		// Update metadata
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		for k, v := range template.Labels {
			configMap.Labels[k] = v
		}
		if configMap.Annotations == nil {
			configMap.Annotations = make(map[string]string)
		}
		for k, v := range template.Annotations {
			configMap.Annotations[k] = v
		}

		return nil
	})

	return err
}

// getServiceTemplate returns a service template for the Batcher component
func (r *OrdererBatcherReconciler) getServiceTemplate(ordererBatcher *fabricxv1alpha1.OrdererBatcher) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getServiceName(ordererBatcher),
			Namespace: ordererBatcher.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port:       r.getServicePort(),
					TargetPort: intstr.FromInt(r.getTargetPort()),
					Protocol:   corev1.ProtocolTCP,
					Name:       "batcher",
				},
			},
			Selector: map[string]string{
				"app":     "batcher",
				"release": ordererBatcher.Name,
			},
		},
	}
}

// updateService updates a service with template data
func (r *OrdererBatcherReconciler) updateService(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher, service *corev1.Service, template *corev1.Service) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ordererBatcher, service, r.Scheme); err != nil {
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

// getDeploymentTemplate returns a deployment template for the Batcher component
func (r *OrdererBatcherReconciler) getDeploymentTemplate(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher) *appsv1.Deployment {
	// Compute ConfigMap hash to trigger deployment updates when config changes
	configMapHash := ""
	configMapName := fmt.Sprintf("%s-config", ordererBatcher.Name)
	hash, err := r.computeConfigMapHash(ctx, configMapName, ordererBatcher.Namespace)
	if err != nil {
		// Log the error but continue with empty hash
		log := logf.FromContext(ctx)
		log.Error(err, "Failed to compute ConfigMap hash, continuing without hash",
			"configMapName", configMapName,
			"namespace", ordererBatcher.Namespace)
	} else {
		configMapHash = hash
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ordererBatcher.Name,
			Namespace: ordererBatcher.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &ordererBatcher.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "batcher",
					"release": ordererBatcher.Name,
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: func() map[string]string {
						labels := map[string]string{
							"app":     "batcher",
							"release": ordererBatcher.Name,
						}
						// Merge with custom pod labels if specified
						if ordererBatcher.Spec.PodLabels != nil {
							for k, v := range ordererBatcher.Spec.PodLabels {
								labels[k] = v
							}
						}
						return labels
					}(),
					Annotations: func() map[string]string {
						annotations := make(map[string]string)
						// Copy existing annotations
						if ordererBatcher.Spec.PodAnnotations != nil {
							for k, v := range ordererBatcher.Spec.PodAnnotations {
								annotations[k] = v
							}
						}
						// Add ConfigMap hash annotation to trigger pod restarts when config changes
						if configMapHash != "" {
							annotations["fabricx.kfsoft.tech/config-hash"] = configMapHash
						}
						return annotations
					}(),
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "setup-msp",
							Image: "busybox:1.35",
							Command: []string{
								"/bin/sh",
								"-c",
								fmt.Sprintf(
									`mkdir -p /%s/msp/signcerts && `+
										"mkdir -p /%s/msp/keystore && "+
										"mkdir -p /%s/msp/cacerts && "+
										"mkdir -p /%s/tls && "+
										"cp /sign-certs/cert.pem /%s/msp/signcerts/ && "+
										"cp /sign-certs/key.pem /%s/msp/keystore/sign-privateKey.pem && "+
										"cp /sign-certs/key.pem /%s/msp/keystore/priv_sk && "+
										"cp /sign-certs/ca.pem /%s/msp/cacerts/ && "+
										"cp /config/msp_config.yaml /%s/msp/config.yaml && "+
										"cp /tls-certs/cert.pem /%s/tls/server.crt && "+
										"cp /tls-certs/key.pem /%s/tls/server.key && "+
										"cp /tls-certs/ca.pem /%s/tls/ca.crt && "+
										"echo 'MSP Directory contents:' && ls -lR /%s/msp && "+
										"echo 'TLS Directory contents:' && ls -lR /%s/tls",
									ordererBatcher.Name, ordererBatcher.Name, ordererBatcher.Name, ordererBatcher.Name,
									ordererBatcher.Name, ordererBatcher.Name, ordererBatcher.Name, ordererBatcher.Name,
									ordererBatcher.Name,
									ordererBatcher.Name, ordererBatcher.Name, ordererBatcher.Name,
									ordererBatcher.Name, ordererBatcher.Name,
								),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									ReadOnly:  true,
									MountPath: "/config",
								},
								{
									Name:      "sign-certs",
									ReadOnly:  true,
									MountPath: "/sign-certs",
								},
								{
									Name:      "tls-certs",
									ReadOnly:  true,
									MountPath: "/tls-certs",
								},
								{
									Name:      "shared-msp",
									MountPath: fmt.Sprintf("/%s", ordererBatcher.Name),
								},
							},
						},
						{
							Name:  "setup-genesis",
							Image: "busybox:1.35",
							Command: []string{
								"/bin/sh",
								"-c",
								fmt.Sprintf(
									"cp /genesis-block/genesis.block /%s/genesis.block",
									ordererBatcher.Name,
								),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "genesis-block",
									ReadOnly:  true,
									MountPath: "/genesis-block",
								},
								{
									Name:      "shared-msp",
									MountPath: fmt.Sprintf("/%s", ordererBatcher.Name),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "batcher",
							Image: fmt.Sprintf("%s:%s",
								func() string {
									if ordererBatcher.Spec.Image != "" {
										return ordererBatcher.Spec.Image
									}
									return "hyperledger/fabric-x-orderer"
								}(),
								func() string {
									if ordererBatcher.Spec.ImageTag != "" {
										return ordererBatcher.Spec.ImageTag
									}
									return "0.0.19"
								}()),
							Args: []string{
								"batcher",
								"--config=/config/node_config.yaml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "batcher-port",
									ContainerPort: 7151,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									ReadOnly:  true,
									MountPath: "/config",
								},
								{
									Name:      "shared-msp",
									MountPath: fmt.Sprintf("/%s", ordererBatcher.Name),
								},
								{
									Name:      "store",
									MountPath: fmt.Sprintf("/%s/store", ordererBatcher.Name),
								},
							},
							Resources: func() corev1.ResourceRequirements {
								if ordererBatcher.Spec.Resources != nil {
									return *ordererBatcher.Spec.Resources
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
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", ordererBatcher.Name),
									},
								},
							},
						},
						{
							Name: "sign-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-sign-cert", ordererBatcher.Name),
								},
							},
						},
						{
							Name: "tls-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-tls-cert", ordererBatcher.Name),
								},
							},
						},
						{
							Name: "shared-msp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "store",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("%s-store-pvc", ordererBatcher.Name),
								},
							},
						},
						{
							Name: "genesis-block",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: ordererBatcher.Spec.Genesis.SecretName,
									Items: []corev1.KeyToPath{
										{
											Key: func() string {
												if ordererBatcher.Spec.Genesis.SecretKey != "" {
													return ordererBatcher.Spec.Genesis.SecretKey
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

	return deployment
}

// updateDeployment updates a deployment with template data
func (r *OrdererBatcherReconciler) updateDeployment(ctx context.Context, ordererBatcher *fabricxv1alpha1.OrdererBatcher, deployment *appsv1.Deployment, template *appsv1.Deployment) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ordererBatcher, deployment, r.Scheme); err != nil {
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

// SetupWithManager sets up the controller with the Manager.
func (r *OrdererBatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register Istio types with the scheme
	if err := istionetworkingv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("failed to add Istio networking v1beta1 to scheme: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.OrdererBatcher{}).
		Named("ordererbatcher").
		Complete(r)
}
