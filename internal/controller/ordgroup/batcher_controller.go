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

package ordgroup

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
)

// BatcherController handles reconciliation for the Batcher component
type BatcherController struct {
	BaseComponentController
}

// NewBatcherController creates a new Batcher controller
func NewBatcherController(client client.Client, scheme *runtime.Scheme) *BatcherController {
	return &BatcherController{
		BaseComponentController: NewBaseComponentController(client, scheme),
	}
}

// NewBatcherControllerWithCertService creates a new Batcher controller with a custom certificate service
func NewBatcherControllerWithCertService(client client.Client, scheme *runtime.Scheme, certService certs.OrdererGroupCertServiceInterface) *BatcherController {
	return &BatcherController{
		BaseComponentController: BaseComponentController{
			Client:      client,
			Scheme:      scheme,
			CertService: certService,
		},
	}
}

// Reconcile reconciles the Batcher component
func (r *BatcherController) Reconcile(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling Batcher components",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace,
		"mode", ordererGroup.Spec.BootstrapMode)

	// Handle different modes
	switch ordererGroup.Spec.BootstrapMode {
	case "configure":
		// In configure mode, only create certificates for all batchers
		for i, batcher := range ordererGroup.Spec.Components.Batchers {
			if err := r.reconcileCertificates(ctx, ordererGroup, fmt.Sprintf("batcher-%d", i), &fabricxv1alpha1.ComponentConfig{
				CommonComponentConfig: batcher.CommonComponentConfig,
				Certificates:          batcher.Certificates,
			}); err != nil {
				return fmt.Errorf("failed to reconcile batcher-%d certificates: %w", i, err)
			}
		}
		log.Info("Batcher certificates created in configure mode")
		return nil

	case "deploy":
		// In deploy mode, create all resources for each batcher instance
		for i, batcher := range ordererGroup.Spec.Components.Batchers {
			batcherName := fmt.Sprintf("batcher-%d", i)
			log.Info("Reconciling batcher instance",
				"name", batcherName,
				"shardID", batcher.ShardID,
				"replicas", batcher.Replicas)

			// Convert BatcherInstance to ComponentConfig for compatibility
			componentConfig := &fabricxv1alpha1.ComponentConfig{
				CommonComponentConfig: batcher.CommonComponentConfig,
				Ingress:               batcher.Ingress,
				Certificates:          batcher.Certificates,
				Endpoints:             batcher.Endpoints,
				Env:                   batcher.Env,
				Command:               batcher.Command,
				Args:                  batcher.Args,
			}

			// 1. Create/Update certificates first
			if err := r.reconcileCertificates(ctx, ordererGroup, batcherName, componentConfig); err != nil {
				return fmt.Errorf("failed to reconcile %s certificates: %w", batcherName, err)
			}

			// 2. Create/Update ConfigMap for Batcher configuration
			if err := r.reconcileConfigMap(ctx, ordererGroup, componentConfig, i, batcher.ShardID); err != nil {
				return fmt.Errorf("failed to reconcile %s configmap: %w", batcherName, err)
			}

			// 3. Create/Update Service for Batcher
			if err := r.reconcileService(ctx, ordererGroup, componentConfig, i); err != nil {
				return fmt.Errorf("failed to reconcile %s service: %w", batcherName, err)
			}

			// 4. Create/Update Deployment for Batcher
			if err := r.reconcileDeployment(ctx, ordererGroup, componentConfig, i, batcher.ShardID); err != nil {
				return fmt.Errorf("failed to reconcile %s deployment: %w", batcherName, err)
			}

			// 5. Create/Update PVC for Batcher
			if err := r.reconcilePVC(ctx, ordererGroup, componentConfig, i); err != nil {
				return fmt.Errorf("failed to reconcile %s PVC: %w", batcherName, err)
			}

			// 6. Create/Update Ingress for Batcher (if configured)
			if batcher.Ingress != nil {
				if err := r.reconcileIngress(ctx, ordererGroup, componentConfig, i); err != nil {
					return fmt.Errorf("failed to reconcile %s ingress: %w", batcherName, err)
				}
			}
		}

	default:
		return fmt.Errorf("unknown bootstrap mode: %s", ordererGroup.Spec.BootstrapMode)
	}

	log.Info("All Batcher components reconciled successfully")
	return nil
}

// Cleanup cleans up the Batcher component resources
func (r *BatcherController) Cleanup(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig) error {
	log := logf.FromContext(ctx)

	log.Info("Cleaning up Batcher component",
		"name", ordererGroup.Name,
		"namespace", ordererGroup.Namespace)

	// Always cleanup certificates regardless of mode
	if err := r.cleanupCertificates(ctx, ordererGroup, "batcher"); err != nil {
		log.Error(err, "Failed to cleanup batcher certificates")
	}

	// Only cleanup other resources in deploy mode
	if ordererGroup.Spec.BootstrapMode == "deploy" {
		// 1. Delete Deployment
		if err := r.cleanupDeployment(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup batcher deployment")
		}

		// 2. Delete Service
		if err := r.cleanupService(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup batcher service")
		}

		// 3. Delete Ingress
		if err := r.cleanupIngress(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup batcher ingress")
		}

		// 4. Delete ConfigMap
		if err := r.cleanupConfigMap(ctx, ordererGroup); err != nil {
			log.Error(err, "Failed to cleanup batcher configmap")
		}
	}

	log.Info("Batcher component cleanup completed")
	return nil
}

// reconcileConfigMap creates or updates the ConfigMap for Batcher configuration
func (r *BatcherController) reconcileConfigMap(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig, instanceIndex int, shardID int32) error {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher-%d-config", ordererGroup.Name, instanceIndex),
			Namespace: ordererGroup.Namespace,
		},
	}

	// Generate batcher configuration based on the example provided
	batcherConfig := fmt.Sprintf(`PartyID: %d
General:
    ListenAddress: 0.0.0.0
    ListenPort: %d
    TLS:
        Enabled: false
        PrivateKey: %s/batcher-%d/tls/server.key
        Certificate: %s/batcher-%d/tls/server.crt
        RootCAs:
            - %s/batcher-%d/tls/ca.crt
        ClientAuthRequired: false
    Keepalive:
        ClientInterval: 1m0s
        ClientTimeout: 20s
        ServerInterval: 2h0m0s
        ServerTimeout: 20s
        ServerMinInterval: 1m0s
    Backoff:
        BaseDelay: 1s
        Multiplier: 1.6
        MaxDelay: 2m0s
    MaxRecvMsgSize: 104857600
    MaxSendMsgSize: 104857600
    Bootstrap:
        Method: block
        File: %s/batcher-%d/genesis.block
    LocalMSPDir: %s/batcher-%d/msp
    LocalMSPID: %s
    LogSpec: info
FileStore:
    Location: %s/batcher-%d/store
Batcher:
    ShardID: %d
    BatchSequenceGap: 12
    MemPoolMaxSize: 1200000
    SubmitTimeout: 600ms`,
		ordererGroup.Spec.PartyID, 7151+instanceIndex, ordererGroup.Name, instanceIndex, ordererGroup.Name, instanceIndex, ordererGroup.Name, instanceIndex,
		ordererGroup.Name, instanceIndex, ordererGroup.Name, instanceIndex, ordererGroup.Spec.MSPID,
		ordererGroup.Name, instanceIndex, shardID)

	template := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher-%d-config", ordererGroup.Name, instanceIndex),
			Namespace: ordererGroup.Namespace,
		},
		Data: map[string]string{
			"node_config.yaml": batcherConfig,
		},
	}

	if err := r.updateConfigMap(ctx, ordererGroup, configMap, template); err != nil {
		log.Error(err, "Failed to update ConfigMap", "name", configMap.Name)
		return fmt.Errorf("failed to update ConfigMap %s: %w", configMap.Name, err)
	}

	log.Info("ConfigMap reconciled successfully", "batcher", ordererGroup.Name)
	return nil
}

// reconcileService creates or updates the Service for Batcher
func (r *BatcherController) reconcileService(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig, instanceIndex int) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher-%d", ordererGroup.Name, instanceIndex),
			Namespace: ordererGroup.Namespace,
		},
	}
	template := r.getServiceTemplate(ordererGroup, instanceIndex)
	if err := r.updateService(ctx, ordererGroup, service, template); err != nil {
		log.Error(err, "Failed to update Service", "name", service.Name)
		return fmt.Errorf("failed to update Service %s: %w", service.Name, err)
	}

	log.Info("Service reconciled successfully", "batcher", ordererGroup.Name)
	return nil
}

// reconcileDeployment creates or updates the Deployment for Batcher
func (r *BatcherController) reconcileDeployment(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig, instanceIndex int, shardID int32) error {
	log := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher-%d", ordererGroup.Name, instanceIndex),
			Namespace: ordererGroup.Namespace,
		},
	}
	template := r.getDeploymentTemplate(ctx, ordererGroup, config, instanceIndex, shardID)
	if err := r.updateDeployment(ctx, ordererGroup, deployment, template); err != nil {
		log.Error(err, "Failed to update Deployment", "name", deployment.Name)
		return fmt.Errorf("failed to update Deployment %s: %w", deployment.Name, err)
	}

	log.Info("Deployment reconciled successfully", "batcher", ordererGroup.Name)
	return nil
}

// reconcilePVC creates or updates the PVC for Batcher
func (r *BatcherController) reconcilePVC(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig, instanceIndex int) error {
	log := logf.FromContext(ctx)

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher-%d-store-pvc", ordererGroup.Name, instanceIndex),
			Namespace: ordererGroup.Namespace,
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
			storageClassName := "fast-ssd"
			pvc.Spec = corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
				StorageClassName: &storageClassName,
			}

			// Set controller reference
			if err := controllerutil.SetControllerReference(ordererGroup, pvc, r.Scheme); err != nil {
				return fmt.Errorf("failed to set controller reference for PVC: %w", err)
			}

			if err := r.Client.Create(ctx, pvc); err != nil {
				return fmt.Errorf("failed to create PVC: %w", err)
			}

			log.Info("Created PVC", "name", pvc.Name, "namespace", pvc.Namespace)
		} else {
			return fmt.Errorf("failed to get PVC: %w", err)
		}
	} else {
		log.Info("PVC already exists", "name", pvc.Name, "namespace", pvc.Namespace)
	}

	return nil
}

// reconcileIngress creates or updates the Ingress for Batcher
func (r *BatcherController) reconcileIngress(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig, instanceIndex int) error {
	// TODO: Implement Ingress reconciliation
	// This would create/update an Ingress resource based on the ingress configuration
	return nil
}

// cleanupDeployment deletes the Batcher Deployment
func (r *BatcherController) cleanupDeployment(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher", ordererGroup.Name),
			Namespace: ordererGroup.Namespace,
		},
	}

	if err := r.Client.Delete(ctx, deployment); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete batcher deployment: %w", err)
		}
	}

	return nil
}

// cleanupService deletes the Batcher Service
func (r *BatcherController) cleanupService(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher", ordererGroup.Name),
			Namespace: ordererGroup.Namespace,
		},
	}

	if err := r.Client.Delete(ctx, service); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete batcher service: %w", err)
		}
	}

	return nil
}

// cleanupIngress deletes the Batcher Ingress
func (r *BatcherController) cleanupIngress(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher", ordererGroup.Name),
			Namespace: ordererGroup.Namespace,
		},
	}

	if err := r.Client.Delete(ctx, ingress); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete batcher ingress: %w", err)
		}
	}

	return nil
}

// cleanupConfigMap deletes the Batcher ConfigMap
func (r *BatcherController) cleanupConfigMap(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup) error {
	// TODO: Implement ConfigMap cleanup
	return nil
}

// getDeploymentTemplate returns a deployment template for the Batcher component
func (r *BatcherController) getDeploymentTemplate(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, config *fabricxv1alpha1.ComponentConfig, instanceIndex int, shardID int32) *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher-%d", ordererGroup.Name, instanceIndex),
			Namespace: ordererGroup.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &config.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     fmt.Sprintf("batcher-%d", instanceIndex),
					"release": ordererGroup.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: func() map[string]string {
						labels := map[string]string{
							"app":     fmt.Sprintf("batcher-%d", instanceIndex),
							"release": ordererGroup.Name,
						}
						// Merge with custom pod labels if specified
						if config.PodLabels != nil {
							for k, v := range config.PodLabels {
								labels[k] = v
							}
						}
						return labels
					}(),
					Annotations: func() map[string]string {
						annotations := make(map[string]string)
						// Copy existing annotations
						if config.PodAnnotations != nil {
							for k, v := range config.PodAnnotations {
								annotations[k] = v
							}
						}
						return annotations
					}(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  fmt.Sprintf("batcher-%d", instanceIndex),
							Image: "hyperledger/fabric-x-committer:0.1.4",
							Command: []string{
								"arma",
							},
							Args: []string{
								"batcher",
								"--config=/config/node_config.yaml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          fmt.Sprintf("batcher-%d-port", instanceIndex),
									ContainerPort: 7151 + int32(instanceIndex),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: func() []corev1.VolumeMount {
								volumeMounts := []corev1.VolumeMount{
									{
										Name:      "config",
										ReadOnly:  true,
										MountPath: "/config",
									},
									{
										Name:      "certs",
										ReadOnly:  true,
										MountPath: fmt.Sprintf("%s/batcher-%d", ordererGroup.Name, instanceIndex),
									},
									{
										Name:      "store",
										MountPath: fmt.Sprintf("%s/batcher-%d/store", ordererGroup.Name, instanceIndex),
									},
								}

								// Automatically add genesis block mount if genesis is configured
								if ordererGroup.Spec.Genesis.SecretName != "" {
									volumeMounts = append(volumeMounts, corev1.VolumeMount{
										Name:      "genesis-block",
										ReadOnly:  true,
										MountPath: fmt.Sprintf("%s/batcher-%d/genesis.block", ordererGroup.Name, instanceIndex),
										SubPath:   ordererGroup.Spec.Genesis.SecretKey,
									})
								}

								return volumeMounts
							}(),
							Resources: func() corev1.ResourceRequirements {
								if config.Resources != nil {
									return *config.Resources
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
					Volumes: func() []corev1.Volume {
						volumes := []corev1.Volume{
							{
								Name: "config",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: fmt.Sprintf("%s-batcher-%d-config", ordererGroup.Name, instanceIndex),
										},
									},
								},
							},
							{
								Name: "certs",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: fmt.Sprintf("%s-batcher-sign-cert", ordererGroup.Name),
									},
								},
							},
							{
								Name: "store",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: fmt.Sprintf("%s-batcher-%d-store-pvc", ordererGroup.Name, instanceIndex),
									},
								},
							},
						}

						// Automatically add genesis block volume if genesis is configured
						if ordererGroup.Spec.Genesis.SecretName != "" {
							volumes = append(volumes, corev1.Volume{
								Name: "genesis-block",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: ordererGroup.Spec.Genesis.SecretName,
									},
								},
							})
						}

						return volumes
					}(),
				},
			},
		},
	}

	// Set environment variables
	if config.Env != nil {
		deployment.Spec.Template.Spec.Containers[0].Env = func() []corev1.EnvVar {
			envVars := make([]corev1.EnvVar, len(config.Env))
			for i, env := range config.Env {
				envVars[i] = corev1.EnvVar{
					Name:  env.Name,
					Value: env.Value,
				}
			}
			return envVars
		}()
	}

	// Set image pull secrets
	if config.ImagePullSecrets != nil {
		deployment.Spec.Template.Spec.ImagePullSecrets = func() []corev1.LocalObjectReference {
			secrets := make([]corev1.LocalObjectReference, len(config.ImagePullSecrets))
			for i, secret := range config.ImagePullSecrets {
				secrets[i] = corev1.LocalObjectReference{
					Name: secret.Name,
				}
			}
			return secrets
		}()
	}

	// Set affinity
	if config.Affinity != nil {
		deployment.Spec.Template.Spec.Affinity = func() *corev1.Affinity {
			affinity := &corev1.Affinity{}
			if config.Affinity.NodeAffinity != nil {
				affinity.NodeAffinity = &corev1.NodeAffinity{}
				if config.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
					affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
					for _, term := range config.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
						nodeSelectorTerm := corev1.NodeSelectorTerm{}
						for _, req := range term.MatchExpressions {
							nodeSelectorTerm.MatchExpressions = append(nodeSelectorTerm.MatchExpressions, corev1.NodeSelectorRequirement{
								Key:      req.Key,
								Operator: corev1.NodeSelectorOperator(req.Operator),
								Values:   req.Values,
							})
						}
						affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(
							affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
							nodeSelectorTerm,
						)
					}
				}
			}
			return affinity
		}()
	}

	// Set tolerations
	if config.Tolerations != nil {
		deployment.Spec.Template.Spec.Tolerations = func() []corev1.Toleration {
			tolerations := make([]corev1.Toleration, len(config.Tolerations))
			for i, tol := range config.Tolerations {
				tolerations[i] = corev1.Toleration{
					Key:               tol.Key,
					Operator:          corev1.TolerationOperator(tol.Operator),
					Value:             tol.Value,
					Effect:            corev1.TaintEffect(tol.Effect),
					TolerationSeconds: tol.TolerationSeconds,
				}
			}
			return tolerations
		}()
	}

	return deployment
}

// updateDeployment updates a deployment with template data
func (r *BatcherController) updateDeployment(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, deployment *appsv1.Deployment, template *appsv1.Deployment) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ordererGroup, deployment, r.Scheme); err != nil {
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

// updateConfigMap updates a configmap with template data
func (r *BatcherController) updateConfigMap(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, configMap *corev1.ConfigMap, template *corev1.ConfigMap) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ordererGroup, configMap, r.Scheme); err != nil {
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
func (r *BatcherController) getServiceTemplate(ordererGroup *fabricxv1alpha1.OrdererGroup, instanceIndex int) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-batcher-%d", ordererGroup.Name, instanceIndex),
			Namespace: ordererGroup.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Port:       7151 + int32(instanceIndex),
					TargetPort: intstr.FromInt(7151 + instanceIndex),
					Protocol:   corev1.ProtocolTCP,
					Name:       fmt.Sprintf("batcher-%d", instanceIndex),
				},
			},
			Selector: map[string]string{
				"app":     fmt.Sprintf("batcher-%d", instanceIndex),
				"release": ordererGroup.Name,
			},
		},
	}
}

// updateService updates a service with template data
func (r *BatcherController) updateService(ctx context.Context, ordererGroup *fabricxv1alpha1.OrdererGroup, service *corev1.Service, template *corev1.Service) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set controller reference
		if err := controllerutil.SetControllerReference(ordererGroup, service, r.Scheme); err != nil {
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
