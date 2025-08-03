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

package ca

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

// CAReconciler reconciles a CA object
type CAReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=cas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=cas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=cas/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

const (
	caFinalizer = "finalizer.ca.fabricx.kfsoft.tech"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *CAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling CA", "namespace", req.Namespace, "name", req.Name)

	// Fetch the CA instance
	ca := &fabricxv1alpha1.CA{}
	err := r.Get(ctx, req.NamespacedName, ca)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			log.Info("CA resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get CA.")
		return ctrl.Result{}, err
	}

	// Check if the CA instance is marked to be deleted
	if ca.GetDeletionTimestamp() != nil {
		return r.handleDeletion(ctx, ca)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(ca, caFinalizer) {
		controllerutil.AddFinalizer(ca, caFinalizer)
		if err := r.Update(ctx, ca); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set default values if not specified
	r.setDefaults(ca)

	// Reconcile the CA resources
	if err := r.reconcileCA(ctx, ca); err != nil {
		log.Error(err, "Failed to reconcile CA")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.updateStatus(ctx, ca); err != nil {
		log.Error(err, "Failed to update CA status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// setDefaults sets default values for the CA spec
func (r *CAReconciler) setDefaults(ca *fabricxv1alpha1.CA) {
	if ca.Spec.Image == "" {
		ca.Spec.Image = "hyperledger/fabric-ca:1.4.3"
	}
	if ca.Spec.Version == "" {
		ca.Spec.Version = "1.4.3"
	}
	if ca.Spec.CredentialStore == "" {
		ca.Spec.CredentialStore = fabricxv1alpha1.CredentialStoreKubernetes
	}
	if ca.Spec.Replicas == nil {
		replicas := int32(1)
		ca.Spec.Replicas = &replicas
	}
	if ca.Spec.Service.ServiceType == "" {
		ca.Spec.Service.ServiceType = corev1.ServiceTypeClusterIP
	}
	if ca.Spec.Storage.AccessMode == "" {
		ca.Spec.Storage.AccessMode = "ReadWriteOnce"
	}
	if ca.Spec.Storage.Size == "" {
		ca.Spec.Storage.Size = "1Gi"
	}
}

// handleDeletion handles the deletion of CA resources
func (r *CAReconciler) handleDeletion(ctx context.Context, ca *fabricxv1alpha1.CA) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Handling CA deletion")

	if controllerutil.ContainsFinalizer(ca, caFinalizer) {
		// Delete all resources
		if err := r.deleteResources(ctx, ca); err != nil {
			log.Error(err, "Failed to delete CA resources")
			return ctrl.Result{}, err
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(ca, caFinalizer)
		if err := r.Update(ctx, ca); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// reconcileCA reconciles all CA resources
func (r *CAReconciler) reconcileCA(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	// Reconcile ConfigMaps
	if err := r.reconcileConfigMaps(ctx, ca); err != nil {
		return err
	}

	// Reconcile Secrets
	if err := r.reconcileSecrets(ctx, ca); err != nil {
		return err
	}

	// Reconcile PVC
	if err := r.reconcilePVC(ctx, ca); err != nil {
		return err
	}

	// Reconcile Deployment
	if err := r.reconcileDeployment(ctx, ca); err != nil {
		return err
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, ca); err != nil {
		return err
	}

	return nil
}

// reconcileConfigMaps reconciles CA ConfigMaps
func (r *CAReconciler) reconcileConfigMaps(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	// Main CA config
	caConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, caConfig, func() error {
		caConfig.Data = map[string]string{
			"ca.yaml": r.generateCAConfig(ca),
		}
		return controllerutil.SetControllerReference(ca, caConfig, r.Scheme)
	})
	if err != nil {
		return err
	}

	// TLS CA config
	tlsConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config-tls", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, tlsConfig, func() error {
		tlsConfig.Data = map[string]string{
			"fabric-ca-server-config.yaml": r.generateTLSCAConfig(ca),
		}
		return controllerutil.SetControllerReference(ca, tlsConfig, r.Scheme)
	})
	if err != nil {
		return err
	}

	// Environment config
	envConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-env", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, envConfig, func() error {
		envConfig.Data = map[string]string{
			"GODEBUG":        "netdns=go",
			"FABRIC_CA_HOME": "/var/hyperledger/fabric-ca",
			"SERVICE_DNS":    "0.0.0.0",
		}
		return controllerutil.SetControllerReference(ca, envConfig, r.Scheme)
	})

	return err
}

// reconcileSecrets reconciles CA Secrets
func (r *CAReconciler) reconcileSecrets(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	// Generate TLS certificates
	tlsCert, tlsKey, err := r.generateTLSCertificate(ca)
	if err != nil {
		return err
	}

	// Generate CA certificates
	caCert, caKey, err := r.generateCACertificate(ca)
	if err != nil {
		return err
	}

	// TLS crypto material secret
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-tls-crypto", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, tlsSecret, func() error {
		tlsSecret.Data = map[string][]byte{
			"tls.crt": tlsCert,
			"tls.key": tlsKey,
		}
		return controllerutil.SetControllerReference(ca, tlsSecret, r.Scheme)
	})
	if err != nil {
		return err
	}

	// MSP crypto material secret
	mspSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-msp-crypto", ca.Name),
			Namespace: ca.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, mspSecret, func() error {
		mspSecret.Data = map[string][]byte{
			"certfile": caCert,
			"keyfile":  caKey,
		}
		return controllerutil.SetControllerReference(ca, mspSecret, r.Scheme)
	})

	return err
}

// reconcilePVC reconciles the PersistentVolumeClaim
func (r *CAReconciler) reconcilePVC(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		pvc.Spec = corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.PersistentVolumeAccessMode(ca.Spec.Storage.AccessMode),
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(ca.Spec.Storage.Size),
				},
			},
		}
		if ca.Spec.Storage.StorageClass != "" && ca.Spec.Storage.StorageClass != "-" {
			pvc.Spec.StorageClassName = &ca.Spec.Storage.StorageClass
		}
		return controllerutil.SetControllerReference(ca, pvc, r.Scheme)
	})

	return err
}

// reconcileDeployment reconciles the CA Deployment
func (r *CAReconciler) reconcileDeployment(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		deployment.Spec = appsv1.DeploymentSpec{
			Replicas: ca.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "ca",
					"release": ca.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "ca",
						"release": ca.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "ca",
							Image: fmt.Sprintf("%s:%s", ca.Spec.Image, ca.Spec.Version),
							Command: []string{
								"sh",
								"-c",
								`mkdir -p $FABRIC_CA_HOME
cp /var/hyperledger/ca_config/ca.yaml $FABRIC_CA_HOME/fabric-ca-server-config.yaml
cp /var/hyperledger/ca_config_tls/fabric-ca-server-config.yaml $FABRIC_CA_HOME/fabric-ca-server-config-tls.yaml
echo ">\033[0;35m fabric-ca-server start \033[0m"
fabric-ca-server start`,
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "ca-port",
									ContainerPort: 7054,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "operations-port",
									ContainerPort: 9443,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/cainfo",
										Port:   intstr.FromInt(7054),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    10,
								SuccessThreshold: 1,
								FailureThreshold: 3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/cainfo",
										Port:   intstr.FromInt(7054),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    10,
								SuccessThreshold: 1,
								FailureThreshold: 3,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/hyperledger",
								},
								{
									Name:      "ca-config",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/ca_config",
								},
								{
									Name:      "ca-config-tls",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/ca_config_tls",
								},
								{
									Name:      "tls-secret",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/tls/secret",
								},
								{
									Name:      "msp-cryptomaterial",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/fabric-ca/msp-secret",
								},
								{
									Name:      "msp-tls-cryptomaterial",
									ReadOnly:  true,
									MountPath: "/var/hyperledger/fabric-ca/msp-tls-secret",
								},
							},
							Resources: ca.Spec.Resources,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: ca.Name,
								},
							},
						},
						{
							Name: "ca-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", ca.Name),
									},
								},
							},
						},
						{
							Name: "ca-config-tls",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config-tls", ca.Name),
									},
								},
							},
						},
						{
							Name: "tls-secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-tls-crypto", ca.Name),
								},
							},
						},
						{
							Name: "msp-cryptomaterial",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-msp-crypto", ca.Name),
								},
							},
						},
						{
							Name: "msp-tls-cryptomaterial",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-msp-crypto", ca.Name),
								},
							},
						},
					},
				},
			},
		}

		// Add environment variables
		if len(ca.Spec.Env) > 0 {
			deployment.Spec.Template.Spec.Containers[0].Env = ca.Spec.Env
		}

		// Add image pull secrets
		if len(ca.Spec.ImagePullSecrets) > 0 {
			deployment.Spec.Template.Spec.ImagePullSecrets = ca.Spec.ImagePullSecrets
		}

		// Add node selector
		if ca.Spec.NodeSelector != nil && len(ca.Spec.NodeSelector.NodeSelectorTerms) > 0 {
			nodeSelector := make(map[string]string)
			for _, term := range ca.Spec.NodeSelector.NodeSelectorTerms {
				for _, req := range term.MatchExpressions {
					if req.Operator == corev1.NodeSelectorOpIn && len(req.Values) > 0 {
						nodeSelector[req.Key] = req.Values[0]
					}
				}
			}
			if len(nodeSelector) > 0 {
				deployment.Spec.Template.Spec.NodeSelector = nodeSelector
			}
		}

		// Add affinity
		if ca.Spec.Affinity != nil {
			deployment.Spec.Template.Spec.Affinity = ca.Spec.Affinity
		}

		// Add tolerations
		if len(ca.Spec.Tolerations) > 0 {
			deployment.Spec.Template.Spec.Tolerations = ca.Spec.Tolerations
		}

		return controllerutil.SetControllerReference(ca, deployment, r.Scheme)
	})

	return err
}

// reconcileService reconciles the CA Service
func (r *CAReconciler) reconcileService(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		service.Spec = corev1.ServiceSpec{
			Type: ca.Spec.Service.ServiceType,
			Ports: []corev1.ServicePort{
				{
					Port:       7054,
					TargetPort: intstr.FromInt(7054),
					Protocol:   corev1.ProtocolTCP,
					Name:       "https",
				},
				{
					Port:       9443,
					TargetPort: intstr.FromInt(9443),
					Protocol:   corev1.ProtocolTCP,
					Name:       "operations",
				},
			},
			Selector: map[string]string{
				"app":     "ca",
				"release": ca.Name,
			},
		}
		return controllerutil.SetControllerReference(ca, service, r.Scheme)
	})

	return err
}

// generateCAConfig generates the CA configuration
func (r *CAReconciler) generateCAConfig(ca *fabricxv1alpha1.CA) string {
	// This is a simplified version. In a real implementation, you would generate
	// the full CA configuration based on the spec
	return fmt.Sprintf(`version: "1.4.9"
port: 7054
debug: %t
tls:
  enabled: true
  certfile: /var/hyperledger/tls/secret/tls.crt
  keyfile: /var/hyperledger/tls/secret/tls.key
  clientauth:
    type: noclientcert
ca:
  name: %s
  keyfile: /var/hyperledger/fabric-ca/msp-secret/keyfile
  certfile: /var/hyperledger/fabric-ca/msp-secret/certfile
db:
  type: %s
  datasource: %s
`, ca.Spec.Debug, ca.Spec.CA.Name, ca.Spec.Database.Type, ca.Spec.Database.Datasource)
}

// generateTLSCAConfig generates the TLS CA configuration
func (r *CAReconciler) generateTLSCAConfig(ca *fabricxv1alpha1.CA) string {
	return fmt.Sprintf(`tls:
  certfile: /var/hyperledger/fabric-ca/msp-tls-secret/certfile
  keyfile: /var/hyperledger/fabric-ca/msp-tls-secret/keyfile
  clientauth:
    type: noclientcert
    enabled: true
ca:
  name: %s
  keyfile: /var/hyperledger/fabric-ca/msp-tls-secret/keyfile
  certfile: /var/hyperledger/fabric-ca/msp-tls-secret/certfile
db:
  type: %s
  datasource: %s
`, ca.Spec.TLSCA.Name, ca.Spec.Database.Type, ca.Spec.Database.Datasource)
}

// generateTLSCertificate generates TLS certificate and key
func (r *CAReconciler) generateTLSCertificate(ca *fabricxv1alpha1.CA) ([]byte, []byte, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Create certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	// Get IP addresses and DNS names
	ips := []net.IP{net.ParseIP("127.0.0.1")}
	dnsNames := []string{}
	for _, host := range ca.Spec.Hosts {
		if ip := net.ParseIP(host); ip != nil {
			ips = append(ips, ip)
		} else {
			dnsNames = append(dnsNames, host)
		}
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{ca.Spec.TLS.Subject.O},
			Country:            []string{ca.Spec.TLS.Subject.C},
			Locality:           []string{ca.Spec.TLS.Subject.L},
			OrganizationalUnit: []string{ca.Spec.TLS.Subject.OU},
			StreetAddress:      []string{ca.Spec.TLS.Subject.ST},
		},
		NotBefore:             time.Now().AddDate(0, 0, -1),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           ips,
		DNSNames:              dnsNames,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	// Encode certificate
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	// Encode private key
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	return certPEM, keyPEM, nil
}

// generateCACertificate generates CA certificate and key
func (r *CAReconciler) generateCACertificate(ca *fabricxv1alpha1.CA) ([]byte, []byte, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Create certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{ca.Spec.CA.CSR.Names[0].O},
			Country:            []string{ca.Spec.CA.CSR.Names[0].C},
			Locality:           []string{ca.Spec.CA.CSR.Names[0].L},
			OrganizationalUnit: []string{ca.Spec.CA.CSR.Names[0].OU},
			CommonName:         ca.Spec.CA.CSR.CN,
		},
		NotBefore:             time.Now().AddDate(0, 0, -1),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		SubjectKeyId:          r.computeSKI(privateKey),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	// Encode certificate
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	// Encode private key
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	return certPEM, keyPEM, nil
}

// computeSKI computes the Subject Key Identifier
func (r *CAReconciler) computeSKI(privKey *ecdsa.PrivateKey) []byte {
	raw := elliptic.Marshal(privKey.Curve, privKey.PublicKey.X, privKey.PublicKey.Y)
	hash := sha256.Sum256(raw)
	return hash[:]
}

// updateStatus updates the CA status
func (r *CAReconciler) updateStatus(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	// Get deployment status
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: ca.Name, Namespace: ca.Namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			ca.Status.Status = fabricxv1alpha1.PendingStatus
		} else {
			return err
		}
	} else {
		if deployment.Status.ReadyReplicas > 0 {
			ca.Status.Status = fabricxv1alpha1.RunningStatus
		} else {
			ca.Status.Status = fabricxv1alpha1.PendingStatus
		}
	}

	// Get service to determine node port
	service := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: ca.Name, Namespace: ca.Namespace}, service)
	if err == nil && len(service.Spec.Ports) > 0 {
		ca.Status.NodePort = int(service.Spec.Ports[0].NodePort)
	}

	return r.Status().Update(ctx, ca)
}

// deleteResources deletes all CA resources
func (r *CAReconciler) deleteResources(ctx context.Context, ca *fabricxv1alpha1.CA) error {
	// Delete deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Delete service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	if err := r.Delete(ctx, service); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Delete PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ca.Name,
			Namespace: ca.Namespace,
		},
	}
	if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Delete ConfigMaps
	configMaps := []string{
		fmt.Sprintf("%s-config", ca.Name),
		fmt.Sprintf("%s-config-tls", ca.Name),
		fmt.Sprintf("%s-env", ca.Name),
	}
	for _, name := range configMaps {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ca.Namespace,
			},
		}
		if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	// Delete Secrets
	secrets := []string{
		fmt.Sprintf("%s-tls-crypto", ca.Name),
		fmt.Sprintf("%s-msp-crypto", ca.Name),
	}
	for _, name := range secrets {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ca.Namespace,
			},
		}
		if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CAReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.CA{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Named("ca").
		Complete(r)
}
