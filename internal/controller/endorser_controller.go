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
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
)

const (
	// EndorserFinalizerName is the name of the finalizer used by Endorser
	EndorserFinalizerName = "endorser.fabricx.kfsoft.tech/finalizer"
)

// EndorserReconciler reconciles an Endorser object
type EndorserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=endorsers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=endorsers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=endorsers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *EndorserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in Endorser reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			endorser := &fabricxv1alpha1.Endorser{}
			if err := r.Get(ctx, req.NamespacedName, endorser); err == nil {
				panicMsg := fmt.Sprintf("Panic in Endorser reconciliation: %v", panicErr)
				r.updateEndorserStatus(ctx, endorser, fabricxv1alpha1.FailedStatus, panicMsg)
			}
		}
	}()

	var endorser fabricxv1alpha1.Endorser
	if err := r.Get(ctx, req.NamespacedName, &endorser); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the Endorser is being deleted
	if !endorser.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &endorser)
	}

	// Set initial status if not set
	if endorser.Status.Status == "" {
		r.updateEndorserStatus(ctx, &endorser, fabricxv1alpha1.PendingStatus, "Initializing Endorser")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, &endorser); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateEndorserStatus(ctx, &endorser, fabricxv1alpha1.FailedStatus, errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the Endorser
	if err := r.reconcileEndorser(ctx, &endorser); err != nil {
		if endorser.Status.Status != fabricxv1alpha1.FailedStatus {
			errorMsg := fmt.Sprintf("Failed to reconcile Endorser: %v", err)
			r.updateEndorserStatus(ctx, &endorser, fabricxv1alpha1.FailedStatus, errorMsg)
		}
		log.Error(err, "Failed to reconcile Endorser")
		return ctrl.Result{}, err
	}

	// Requeue after 1 minute to ensure continuous monitoring
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileEndorser handles the reconciliation of an Endorser
func (r *EndorserReconciler) reconcileEndorser(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
	log := logf.FromContext(ctx)

	log.Info("Starting Endorser reconciliation",
		"name", endorser.Name,
		"namespace", endorser.Namespace,
		"bootstrapMode", endorser.Spec.BootstrapMode)

	// Check bootstrap mode - default to configure
	bootstrapMode := endorser.Spec.BootstrapMode
	if bootstrapMode == "" {
		bootstrapMode = "configure"
	}

	// Reconcile based on deployment mode
	switch bootstrapMode {
	case "configure":
		if err := r.reconcileConfigureMode(ctx, endorser); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in configure mode: %v", err)
			log.Error(err, "Failed to reconcile in configure mode")
			r.updateEndorserStatus(ctx, endorser, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in configure mode: %w", err)
		}
	case "deploy":
		if err := r.reconcileDeployMode(ctx, endorser); err != nil {
			errorMsg := fmt.Sprintf("Failed to reconcile in deploy mode: %v", err)
			log.Error(err, "Failed to reconcile in deploy mode")
			r.updateEndorserStatus(ctx, endorser, fabricxv1alpha1.FailedStatus, errorMsg)
			return fmt.Errorf("failed to reconcile in deploy mode: %w", err)
		}
	default:
		errorMsg := fmt.Sprintf("Invalid bootstrap mode: %s", bootstrapMode)
		log.Error(fmt.Errorf("%s", errorMsg), "Invalid bootstrap mode")
		r.updateEndorserStatus(ctx, endorser, fabricxv1alpha1.FailedStatus, errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}

	// Update status to success
	r.updateEndorserStatus(ctx, endorser, fabricxv1alpha1.RunningStatus, "Endorser reconciled successfully")

	log.Info("Endorser reconciliation completed successfully")
	return nil
}

// reconcileConfigureMode handles reconciliation in configure mode (only certificates)
func (r *EndorserReconciler) reconcileConfigureMode(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling Endorser in configure mode",
		"name", endorser.Name,
		"namespace", endorser.Namespace)

	// In configure mode, only create certificates
	if err := r.reconcileCertificates(ctx, endorser); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}
	log.Info("Endorser certificates created in configure mode")

	log.Info("Endorser configure mode reconciliation completed")
	return nil
}

// reconcileDeployMode handles reconciliation in deploy mode (full deployment)
func (r *EndorserReconciler) reconcileDeployMode(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling Endorser in deploy mode",
		"name", endorser.Name,
		"namespace", endorser.Namespace)

	// 1. Reconcile certificates
	if err := r.reconcileCertificates(ctx, endorser); err != nil {
		return fmt.Errorf("failed to reconcile certificates: %w", err)
	}

	// 2. Reconcile core.yaml configuration secret
	if err := r.reconcileCoreConfigSecret(ctx, endorser); err != nil {
		return fmt.Errorf("failed to reconcile core config secret: %w", err)
	}

	// 3. Reconcile Service
	if err := r.reconcileService(ctx, endorser); err != nil {
		return fmt.Errorf("failed to reconcile service: %w", err)
	}

	// 4. Reconcile Deployment
	if err := r.reconcileDeployment(ctx, endorser); err != nil {
		return fmt.Errorf("failed to reconcile deployment: %w", err)
	}

	log.Info("Endorser deploy mode reconciliation completed")
	return nil
}

// reconcileCertificates creates or updates certificates for the Endorser
func (r *EndorserReconciler) reconcileCertificates(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
	log := logf.FromContext(ctx)

	// Check if enrollment is configured
	if endorser.Spec.Enrollment == nil {
		log.Info("No enrollment configuration found, skipping certificate creation")
		return nil
	}

	var allCertificates []certs.ComponentCertificateData

	// 1. Create sign certificate (if configured)
	if endorser.Spec.Enrollment.Sign != nil {
		signCertConfig := &fabricxv1alpha1.CertificateConfig{
			CA: endorser.Spec.Enrollment.Sign.CA,
		}
		signRequest := certs.OrdererGroupCertificateRequest{
			ComponentName:    endorser.Name,
			ComponentType:    "endorser",
			Namespace:        endorser.Namespace,
			CertConfig:       convertToCertConfig(endorser.Spec.MSPID, signCertConfig, "sign"),
			EnrollmentConfig: convertToEnrollmentConfig(endorser.Spec.MSPID, endorser.Spec.Enrollment),
		}
		signCertData, err := certs.CreateSignCertificate(ctx, r.Client, signRequest)
		if err != nil {
			return fmt.Errorf("failed to create sign certificate: %w", err)
		}
		if signCertData != nil {
			allCertificates = append(allCertificates, *signCertData)
		}
	}

	// 2. Create TLS certificate (if configured)
	if endorser.Spec.Enrollment.TLS != nil {
		tlsCertConfig := &fabricxv1alpha1.CertificateConfig{
			CA: endorser.Spec.Enrollment.TLS.CA,
		}
		// Use component-specific SANS if available, otherwise use enrollment SANS
		if endorser.Spec.SANS != nil {
			tlsCertConfig.SANS = endorser.Spec.SANS
		} else if endorser.Spec.Enrollment.TLS.SANS != nil {
			tlsCertConfig.SANS = endorser.Spec.Enrollment.TLS.SANS
		}

		tlsRequest := certs.OrdererGroupCertificateRequest{
			ComponentName:    endorser.Name,
			ComponentType:    "endorser",
			Namespace:        endorser.Namespace,
			CertConfig:       convertToCertConfig(endorser.Spec.MSPID, tlsCertConfig, "tls"),
			EnrollmentConfig: convertToEnrollmentConfig(endorser.Spec.MSPID, endorser.Spec.Enrollment),
		}
		tlsCertData, err := certs.CreateTLSCertificate(ctx, r.Client, tlsRequest)
		if err != nil {
			return fmt.Errorf("failed to create TLS certificate: %w", err)
		}
		if tlsCertData != nil {
			allCertificates = append(allCertificates, *tlsCertData)
		}
	}

	// 3. Create Kubernetes secrets for certificates
	if len(allCertificates) > 0 {
		if err := r.createCertificateSecrets(ctx, endorser, allCertificates); err != nil {
			return fmt.Errorf("failed to create certificate secrets: %w", err)
		}
	}

	log.Info("Certificates reconciled successfully", "endorser", endorser.Name)
	return nil
}

// reconcileCoreConfigSecret creates or updates the core.yaml configuration secret
func (r *EndorserReconciler) reconcileCoreConfigSecret(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
	log := logf.FromContext(ctx)

	// Generate core.yaml content from typed configuration
	coreYAML, err := endorser.GenerateCoreYAML()
	if err != nil {
		return fmt.Errorf("failed to generate core.yaml: %w", err)
	}

	secretName := endorser.GetCoreConfigSecretName()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: endorser.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"core.yaml": []byte(coreYAML),
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(endorser, secret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for core config secret: %w", err)
	}

	// Create or update secret
	existingSecret := &corev1.Secret{}
	err = r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: endorser.Namespace}, existingSecret)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, secret); err != nil {
				return fmt.Errorf("failed to create core config secret: %w", err)
			}
			log.Info("Core config secret created", "secretName", secretName)
		} else {
			return fmt.Errorf("failed to get core config secret: %w", err)
		}
	} else {
		existingSecret.Data = secret.Data
		if err := r.Update(ctx, existingSecret); err != nil {
			return fmt.Errorf("failed to update core config secret: %w", err)
		}
		log.Info("Core config secret updated", "secretName", secretName)
	}

	// Update status with core config secret name
	endorser.Status.CoreConfigSecretName = secretName

	return nil
}

// reconcileService creates or updates the Service for the Endorser
func (r *EndorserReconciler) reconcileService(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
	log := logf.FromContext(ctx)

	serviceName := endorser.GetServiceName()

	// Build service ports from spec or use defaults
	var servicePorts []corev1.ServicePort
	if len(endorser.Spec.Ports) > 0 {
		servicePorts = make([]corev1.ServicePort, 0, len(endorser.Spec.Ports))
		for _, p := range endorser.Spec.Ports {
			protocol := corev1.ProtocolTCP
			if p.Protocol != "" {
				protocol = corev1.Protocol(p.Protocol)
			}
			servicePorts = append(servicePorts, corev1.ServicePort{
				Name:       p.Name,
				Port:       p.Port,
				TargetPort: intstr.FromInt(int(p.Port)),
				Protocol:   protocol,
			})
		}
	} else {
		// Default ports for backward compatibility
		servicePorts = []corev1.ServicePort{
			{
				Name:       "p2p",
				Port:       9301,
				TargetPort: intstr.FromInt(9301),
				Protocol:   corev1.ProtocolTCP,
			},
			{
				Name:       "metrics",
				Port:       9090,
				TargetPort: intstr.FromInt(9090),
				Protocol:   corev1.ProtocolTCP,
			},
		}
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: endorser.Namespace,
			Labels: map[string]string{
				"app":       endorser.Name,
				"component": "endorser",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": endorser.Name,
			},
			Ports: servicePorts,
			Type:  corev1.ServiceTypeClusterIP,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(endorser, service, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for service: %w", err)
	}

	// Create or update service
	existingService := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: endorser.Namespace}, existingService)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, service); err != nil {
				return fmt.Errorf("failed to create service: %w", err)
			}
			log.Info("Service created", "serviceName", serviceName)
		} else {
			return fmt.Errorf("failed to get service: %w", err)
		}
	} else {
		existingService.Spec.Ports = service.Spec.Ports
		existingService.Spec.Selector = service.Spec.Selector
		if err := r.Update(ctx, existingService); err != nil {
			return fmt.Errorf("failed to update service: %w", err)
		}
		log.Info("Service updated", "serviceName", serviceName)
	}

	// Update status with service endpoint
	endorser.Status.ServiceEndpoint = fmt.Sprintf("%s.%s.svc.cluster.local:9301", serviceName, endorser.Namespace)
	endorser.Status.P2PEndpoint = fmt.Sprintf("%s:9301", serviceName)

	return nil
}

// reconcileDeployment creates or updates the Deployment for the Endorser
func (r *EndorserReconciler) reconcileDeployment(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
	log := logf.FromContext(ctx)

	replicas := int32(1)
	if endorser.Spec.Common != nil && endorser.Spec.Common.Replicas > 0 {
		replicas = endorser.Spec.Common.Replicas
	}

	deploymentName := endorser.GetDeploymentName()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: endorser.Namespace,
			Labels: map[string]string{
				"app":       endorser.Name,
				"component": "endorser",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": endorser.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       endorser.Name,
						"component": "endorser",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{},
					Containers: []corev1.Container{
						{
							Name:    "endorser",
							Image:   endorser.GetFullImage(),
							Command: endorser.Spec.Command,
							Args:    endorser.Spec.Args,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "core-config",
									MountPath: "/var/hyperledger/fabric/config",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "core-config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: endorser.GetCoreConfigSecretName(),
								},
							},
						},
					},
				},
			},
		},
	}

	// Mount enrollment certificates from secrets
	if endorser.Status.CertificateSecrets != nil {
		// Mount sign certificate if available
		if signSecretName, ok := endorser.Status.CertificateSecrets["sign"]; ok {
			deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
				Name: "sign-cert",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: signSecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  "cert.pem",
								Path: "cert.pem",
							},
							{
								Key:  "key.pem",
								Path: "key.pem",
							},
							{
								Key:  "ca.pem",
								Path: "ca.pem",
							},
						},
					},
				},
			})

			// Add shared MSP volume for init container setup
			deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
				Name: "shared-msp",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})

			// Add init container to setup MSP directory structure
			mspBasePath := "/var/hyperledger/fabric/config/keys/fabric/user"
			deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, corev1.Container{
				Name:  "setup-msp",
				Image: "busybox:1.35",
				Command: []string{
					"/bin/sh",
					"-c",
					fmt.Sprintf(
						"echo 'Creating MSP directory at: %s' && "+
							"mkdir -p %s/signcerts && "+
							"mkdir -p %s/keystore && "+
							"mkdir -p %s/cacerts && "+
							"echo 'Copying certificates...' && "+
							"ls -l /sign-certs/ && "+
							"cp /sign-certs/cert.pem %s/signcerts/cert.pem && "+
							"cp /sign-certs/key.pem %s/keystore/key.pem && "+
							"cp /sign-certs/key.pem %s/keystore/priv_sk && "+
							"cp /sign-certs/ca.pem %s/cacerts/ca.pem && "+
							"echo 'Creating config.yaml...' && "+
							"cat > %s/config.yaml <<'MSPEOF'\n"+
							"NodeOUs:\n"+
							"  Enable: true\n"+
							"  ClientOUIdentifier:\n"+
							"    Certificate: cacerts/ca.pem\n"+
							"    OrganizationalUnitIdentifier: client\n"+
							"  PeerOUIdentifier:\n"+
							"    Certificate: cacerts/ca.pem\n"+
							"    OrganizationalUnitIdentifier: peer\n"+
							"  AdminOUIdentifier:\n"+
							"    Certificate: cacerts/ca.pem\n"+
							"    OrganizationalUnitIdentifier: admin\n"+
							"  OrdererOUIdentifier:\n"+
							"    Certificate: cacerts/ca.pem\n"+
							"    OrganizationalUnitIdentifier: orderer\n"+
							"MSPEOF\n"+
							"echo 'MSP Directory contents:' && ls -lR /var/hyperledger/fabric/config/keys",
						mspBasePath,
						mspBasePath, mspBasePath, mspBasePath,
						mspBasePath, mspBasePath, mspBasePath, mspBasePath,
						mspBasePath,
					),
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "sign-cert",
						ReadOnly:  true,
						MountPath: "/sign-certs",
					},
					{
						Name:      "shared-msp",
						MountPath: "/var/hyperledger/fabric/config/keys",
					},
				},
			})
			// Mount shared MSP directory for main container
			deployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
				deployment.Spec.Template.Spec.Containers[0].VolumeMounts,
				corev1.VolumeMount{
					Name:      "shared-msp",
					MountPath: "/var/hyperledger/fabric/config/keys",
					ReadOnly:  true,
				},
				corev1.VolumeMount{
					Name:      "sign-cert",
					MountPath: "/var/hyperledger/fabric/msp/signcerts/cert.pem",
					SubPath:   "cert.pem",
					ReadOnly:  true,
				},
				corev1.VolumeMount{
					Name:      "sign-cert",
					MountPath: "/var/hyperledger/fabric/msp/keystore/key.pem",
					SubPath:   "key.pem",
					ReadOnly:  true,
				},
			)
		}

		// Mount TLS certificate if available
		if tlsSecretName, ok := endorser.Status.CertificateSecrets["tls"]; ok {
			deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tlsSecretName,
					},
				},
			})
			deployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
				deployment.Spec.Template.Spec.Containers[0].VolumeMounts,
				corev1.VolumeMount{
					Name:      "tls-cert",
					MountPath: "/var/hyperledger/fabric/tls",
					ReadOnly:  true,
				},
			)
		}
	}

	// Mount resolver identity certificates from secrets
	if endorser.Spec.Core.FSC.Endpoint != nil && len(endorser.Spec.Core.FSC.Endpoint.Resolvers) > 0 {
		for _, resolver := range endorser.Spec.Core.FSC.Endpoint.Resolvers {
			if resolver.Identity != nil && resolver.Identity.SecretRef != nil {
				// Check if the secret exists - REQUIRED
				secretNamespace := resolver.Identity.SecretRef.Namespace
				if secretNamespace == "" {
					secretNamespace = endorser.Namespace
				}

				secret := &corev1.Secret{}
				err := r.Get(ctx, client.ObjectKey{
					Name:      resolver.Identity.SecretRef.Name,
					Namespace: secretNamespace,
				}, secret)

				if err != nil {
					// Secret not found or error - fail reconciliation
					return fmt.Errorf("resolver '%s' certificate secret not found: %s/%s - %w",
						resolver.Name, secretNamespace, resolver.Identity.SecretRef.Name, err)
				}

				// Mount the secret
				volumeName := fmt.Sprintf("resolver-%s", resolver.Name)
				deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
					Name: volumeName,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: resolver.Identity.SecretRef.Name,
							Items: []corev1.KeyToPath{
								{
									Key:  resolver.Identity.SecretRef.Key,
									Path: "cert.pem",
								},
							},
						},
					},
				})
				deployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
					deployment.Spec.Template.Spec.Containers[0].VolumeMounts,
					corev1.VolumeMount{
						Name:      volumeName,
						MountPath: fmt.Sprintf("/var/hyperledger/fabric/resolvers/%s", resolver.Name),
						ReadOnly:  true,
					},
				)
			}
		}
	}

	// Add custom volumes if specified
	if endorser.Spec.Common != nil && len(endorser.Spec.Common.Volumes) > 0 {
		for _, vol := range endorser.Spec.Common.Volumes {
			deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, convertVolume(vol))
		}
	}

	// Idemix credentials are now managed through the Identity CRD
	// Endorsers can mount idemix secrets through the Common.volumeMounts field

	// Add custom volume mounts if specified
	if endorser.Spec.Common != nil && len(endorser.Spec.Common.VolumeMounts) > 0 {
		for _, vm := range endorser.Spec.Common.VolumeMounts {
			deployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
				deployment.Spec.Template.Spec.Containers[0].VolumeMounts,
				convertVolumeMount(vm),
			)
		}
	}

	// Add resource requirements if specified
	if endorser.Spec.Common != nil && endorser.Spec.Common.Resources != nil {
		deployment.Spec.Template.Spec.Containers[0].Resources = *endorser.Spec.Common.Resources
	}

	// Add ports if specified, otherwise use defaults
	if len(endorser.Spec.Ports) > 0 {
		ports := make([]corev1.ContainerPort, 0, len(endorser.Spec.Ports))
		for _, p := range endorser.Spec.Ports {
			protocol := corev1.ProtocolTCP
			if p.Protocol != "" {
				protocol = corev1.Protocol(p.Protocol)
			}
			ports = append(ports, corev1.ContainerPort{
				Name:          p.Name,
				ContainerPort: p.Port,
				Protocol:      protocol,
			})
		}
		deployment.Spec.Template.Spec.Containers[0].Ports = ports
	} else {
		// Default ports for backward compatibility
		deployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
			{
				Name:          "p2p",
				ContainerPort: 9301,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "metrics",
				ContainerPort: 9090,
				Protocol:      corev1.ProtocolTCP,
			},
		}
	}

	// Initialize pod annotations map
	if deployment.Spec.Template.ObjectMeta.Annotations == nil {
		deployment.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}

	// Add hash annotations for all mounted secrets to trigger pod restart on config changes
	if err := r.addConfigHashAnnotations(ctx, endorser, deployment); err != nil {
		log.Error(err, "Failed to add config hash annotations, continuing without them")
	}

	// Add pod annotations if specified (merge with existing annotations)
	if endorser.Spec.Common != nil && len(endorser.Spec.Common.PodAnnotations) > 0 {
		for k, v := range endorser.Spec.Common.PodAnnotations {
			deployment.Spec.Template.ObjectMeta.Annotations[k] = v
		}
	}

	// Add pod labels if specified
	if endorser.Spec.Common != nil && len(endorser.Spec.Common.PodLabels) > 0 {
		for k, v := range endorser.Spec.Common.PodLabels {
			deployment.Spec.Template.ObjectMeta.Labels[k] = v
		}
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(endorser, deployment, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for deployment: %w", err)
	}

	// Create or update deployment
	existingDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: endorser.Namespace}, existingDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, deployment); err != nil {
				return fmt.Errorf("failed to create deployment: %w", err)
			}
			log.Info("Deployment created", "deploymentName", deploymentName)
		} else {
			return fmt.Errorf("failed to get deployment: %w", err)
		}
	} else {
		existingDeployment.Spec = deployment.Spec
		if err := r.Update(ctx, existingDeployment); err != nil {
			return fmt.Errorf("failed to update deployment: %w", err)
		}
		log.Info("Deployment updated", "deploymentName", deploymentName)
	}

	return nil
}

// addConfigHashAnnotations adds hash annotations for all mounted secrets/configmaps
// This ensures pods are restarted when configuration changes
func (r *EndorserReconciler) addConfigHashAnnotations(ctx context.Context, endorser *fabricxv1alpha1.Endorser, deployment *appsv1.Deployment) error {
	annotations := deployment.Spec.Template.ObjectMeta.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Hash core-config secret
	coreConfigSecret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      endorser.GetCoreConfigSecretName(),
		Namespace: endorser.Namespace,
	}, coreConfigSecret); err == nil {
		hash := hashSecretData(coreConfigSecret.Data)
		annotations["fabric-x.kfsoft.tech/core-config-hash"] = hash
	}

	// Hash sign certificate secret
	if endorser.Status.CertificateSecrets != nil {
		if signSecretName, ok := endorser.Status.CertificateSecrets["sign"]; ok {
			signSecret := &corev1.Secret{}
			if err := r.Get(ctx, client.ObjectKey{
				Name:      signSecretName,
				Namespace: endorser.Namespace,
			}, signSecret); err == nil {
				hash := hashSecretData(signSecret.Data)
				annotations["fabric-x.kfsoft.tech/sign-cert-hash"] = hash
			}
		}

		// Hash TLS certificate secret
		if tlsSecretName, ok := endorser.Status.CertificateSecrets["tls"]; ok {
			tlsSecret := &corev1.Secret{}
			if err := r.Get(ctx, client.ObjectKey{
				Name:      tlsSecretName,
				Namespace: endorser.Namespace,
			}, tlsSecret); err == nil {
				hash := hashSecretData(tlsSecret.Data)
				annotations["fabric-x.kfsoft.tech/tls-cert-hash"] = hash
			}
		}
	}

	deployment.Spec.Template.ObjectMeta.Annotations = annotations
	return nil
}

// hashSecretData computes a hash of secret data for change detection
func hashSecretData(data map[string][]byte) string {
	// Import crypto/sha256 at the top of the file
	hash := sha256.New()

	// Sort keys for consistent hashing
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Hash each key-value pair
	for _, k := range keys {
		hash.Write([]byte(k))
		hash.Write(data[k])
	}

	return fmt.Sprintf("%x", hash.Sum(nil))[:16]
}

// ensureFinalizer ensures the finalizer is present on the Endorser
func (r *EndorserReconciler) ensureFinalizer(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
	if !controllerutil.ContainsFinalizer(endorser, EndorserFinalizerName) {
		controllerutil.AddFinalizer(endorser, EndorserFinalizerName)
		if err := r.Update(ctx, endorser); err != nil {
			return fmt.Errorf("failed to add finalizer: %w", err)
		}
	}
	return nil
}

// handleDeletion handles the deletion of an Endorser
func (r *EndorserReconciler) handleDeletion(ctx context.Context, endorser *fabricxv1alpha1.Endorser) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(endorser, EndorserFinalizerName) {
		log.Info("Cleaning up Endorser resources")

		// Perform cleanup (Kubernetes will automatically delete owned resources)
		// Add any custom cleanup logic here if needed

		// Remove finalizer
		controllerutil.RemoveFinalizer(endorser, EndorserFinalizerName)
		if err := r.Update(ctx, endorser); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// updateEndorserStatus updates the status of the Endorser
func (r *EndorserReconciler) updateEndorserStatus(ctx context.Context, endorser *fabricxv1alpha1.Endorser, status fabricxv1alpha1.DeploymentStatus, message string) {
	log := logf.FromContext(ctx)

	endorser.Status.Status = status
	endorser.Status.Message = message

	if err := r.Status().Update(ctx, endorser); err != nil {
		log.Error(err, "Failed to update Endorser status")
	}
}

// convertVolume converts API Volume type to Kubernetes core Volume type
func convertVolume(apiVol fabricxv1alpha1.Volume) corev1.Volume {
	vol := corev1.Volume{
		Name: apiVol.Name,
	}

	// Convert volume source
	if apiVol.VolumeSource.EmptyDir != nil {
		vol.VolumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{
			Medium: corev1.StorageMedium(apiVol.VolumeSource.EmptyDir.Medium),
		}
		// Note: SizeLimit conversion would require parsing the string to resource.Quantity
	}

	if apiVol.VolumeSource.ConfigMap != nil {
		vol.VolumeSource.ConfigMap = &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: apiVol.VolumeSource.ConfigMap.Name,
			},
			DefaultMode: apiVol.VolumeSource.ConfigMap.DefaultMode,
		}
		for _, item := range apiVol.VolumeSource.ConfigMap.Items {
			vol.VolumeSource.ConfigMap.Items = append(vol.VolumeSource.ConfigMap.Items, corev1.KeyToPath{
				Key:  item.Key,
				Path: item.Path,
				Mode: item.Mode,
			})
		}
	}

	if apiVol.VolumeSource.Secret != nil {
		vol.VolumeSource.Secret = &corev1.SecretVolumeSource{
			SecretName:  apiVol.VolumeSource.Secret.SecretName,
			DefaultMode: apiVol.VolumeSource.Secret.DefaultMode,
		}
		for _, item := range apiVol.VolumeSource.Secret.Items {
			vol.VolumeSource.Secret.Items = append(vol.VolumeSource.Secret.Items, corev1.KeyToPath{
				Key:  item.Key,
				Path: item.Path,
				Mode: item.Mode,
			})
		}
	}

	if apiVol.VolumeSource.PersistentVolumeClaim != nil {
		vol.VolumeSource.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: apiVol.VolumeSource.PersistentVolumeClaim.ClaimName,
			ReadOnly:  apiVol.VolumeSource.PersistentVolumeClaim.ReadOnly,
		}
	}

	if apiVol.VolumeSource.HostPath != nil {
		vol.VolumeSource.HostPath = &corev1.HostPathVolumeSource{
			Path: apiVol.VolumeSource.HostPath.Path,
		}
		if apiVol.VolumeSource.HostPath.Type != "" {
			hpType := corev1.HostPathType(apiVol.VolumeSource.HostPath.Type)
			vol.VolumeSource.HostPath.Type = &hpType
		}
	}

	return vol
}

// convertVolumeMount converts API VolumeMount type to Kubernetes core VolumeMount type
func convertVolumeMount(apiVM fabricxv1alpha1.VolumeMount) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apiVM.Name,
		MountPath: apiVM.MountPath,
		ReadOnly:  apiVM.ReadOnly,
		SubPath:   apiVM.SubPath,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *EndorserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.Endorser{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Named("endorser").
		Complete(r)
}
