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
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/hyperledger/fabric-config/protolator"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/genesis"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
	pkgerrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// GenesisFinalizerName is the name of the finalizer used by Genesis
	GenesisFinalizerName = "genesis.fabricx.kfsoft.tech/finalizer"
)

// decodeProto decodes a protobuf message and converts it to JSON
func decodeProto(msgName string, input, output *os.File) error {
	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(msgName))
	if err != nil {
		return pkgerrors.Wrapf(err, "error encode input")
	}

	msgType := reflect.TypeOf(mt.Zero().Interface())

	if msgType == nil {
		return pkgerrors.Errorf("message of type %s unknown", msgType)
	}
	msg := reflect.New(msgType.Elem()).Interface().(proto.Message)

	in, err := io.ReadAll(input)
	if err != nil {
		return pkgerrors.Wrapf(err, "error reading input")
	}

	err = proto.Unmarshal(in, msg)
	if err != nil {
		return pkgerrors.Wrapf(err, "error unmarshalling")
	}

	err = protolator.DeepMarshalJSON(output, msg)
	if err != nil {
		return pkgerrors.Wrapf(err, "error encoding output")
	}

	return nil
}

// GenesisReconciler reconciles a Genesis object
type GenesisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=geneses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=geneses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=geneses/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=cas,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GenesisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in Genesis reconciliation",
				"namespace", req.Namespace, "name", req.Name)

			// Try to update the Genesis status to failed
			genesisCR := &fabricxv1alpha1.Genesis{}
			if err := r.Get(ctx, req.NamespacedName, genesisCR); err == nil {
				panicMsg := fmt.Sprintf("Panic in Genesis reconciliation: %v", panicErr)
				r.updateGenesisStatus(ctx, genesisCR, "FAILED", panicMsg)
			}
		}
	}()

	// Fetch the Genesis instance
	genesisCR := &fabricxv1alpha1.Genesis{}
	err := r.Get(ctx, req.NamespacedName, genesisCR)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			log.Info("Genesis resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Genesis")
		return ctrl.Result{}, err
	}

	log.Info("Reconciling Genesis",
		"name", genesisCR.Name,
		"namespace", genesisCR.Namespace,
		"channelID", genesisCR.Spec.ChannelID)

	// Check if the Genesis is being deleted
	if !genesisCR.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, genesisCR)
	}

	// Set initial status if not set
	if genesisCR.Status.Status == "" {
		r.updateGenesisStatus(ctx, genesisCR, "PENDING", "Initializing Genesis")
	}

	// Ensure finalizer is present
	if err := r.ensureFinalizer(ctx, genesisCR); err != nil {
		errorMsg := fmt.Sprintf("Failed to ensure finalizer: %v", err)
		log.Error(err, "Failed to ensure finalizer")
		r.updateGenesisStatus(ctx, genesisCR, "FAILED", errorMsg)
		return ctrl.Result{}, err
	}

	// Reconcile the Genesis
	if err := r.reconcileGenesis(ctx, genesisCR); err != nil {
		// The reconcileGenesis method should have already updated the status
		// but we'll ensure it's set to failed if it's not already
		if genesisCR.Status.Status != "FAILED" {
			errorMsg := fmt.Sprintf("Failed to reconcile Genesis: %v", err)
			r.updateGenesisStatus(ctx, genesisCR, "FAILED", errorMsg)
		}
		log.Error(err, "Failed to reconcile Genesis")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileGenesis handles the reconciliation of a Genesis
func (r *GenesisReconciler) reconcileGenesis(ctx context.Context, genesisCR *fabricxv1alpha1.Genesis) error {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in Genesis reconciliation",
				"genesis", genesisCR.Name, "namespace", genesisCR.Namespace)

			// Update the Genesis status to failed
			panicMsg := fmt.Sprintf("Panic in Genesis reconciliation: %v", panicErr)
			r.updateGenesisStatus(ctx, genesisCR, "FAILED", panicMsg)
		}
	}()

	log.Info("Starting Genesis reconciliation",
		"name", genesisCR.Name,
		"namespace", genesisCR.Namespace,
		"channelID", genesisCR.Spec.ChannelID)

	// Validate Genesis spec before proceeding
	if err := r.validateGenesisSpec(genesisCR); err != nil {
		errorMsg := fmt.Sprintf("Invalid Genesis spec: %v", err)
		log.Error(err, "Genesis spec validation failed")
		r.updateGenesisStatus(ctx, genesisCR, "FAILED", errorMsg)
		return fmt.Errorf("genesis spec validation failed: %w", err)
	}

	// Create a logger for the Genesis service
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Create Genesis service
	genesisService := genesis.NewGenesisService(r.Client, logger, genesisCR.Spec.ChannelID)

	// Create genesis block with additional error handling
	genesisBlock, err := genesisService.CreateGenesisBlock(ctx, &genesis.GenesisRequest{
		Genesis:   genesisCR,
		ChannelID: genesisCR.Spec.ChannelID,
	})
	if err != nil {
		errorMsg := fmt.Sprintf("Failed to create genesis block: %v", err)
		log.Error(err, "Failed to create genesis block")
		r.updateGenesisStatus(ctx, genesisCR, "FAILED", errorMsg)
		return fmt.Errorf("failed to create genesis block: %w", err)
	}

	// Validate that genesis block is not nil or empty
	if genesisBlock == nil {
		errorMsg := "Generated genesis block is nil"
		log.Error(nil, "Generated genesis block is nil")
		r.updateGenesisStatus(ctx, genesisCR, "FAILED", errorMsg)
		return fmt.Errorf("generated genesis block is nil")
	}

	if len(genesisBlock) == 0 {
		errorMsg := "Generated genesis block is empty"
		log.Error(nil, "Generated genesis block is empty")
		r.updateGenesisStatus(ctx, genesisCR, "FAILED", errorMsg)
		return fmt.Errorf("generated genesis block is empty")
	}

	// Store genesis block in Kubernetes Secret
	if err := genesisService.StoreGenesisBlock(ctx, genesisCR, genesisBlock); err != nil {
		errorMsg := fmt.Sprintf("Failed to store genesis block: %v", err)
		log.Error(err, "Failed to store genesis block")
		r.updateGenesisStatus(ctx, genesisCR, "FAILED", errorMsg)
		return fmt.Errorf("failed to store genesis block: %w", err)
	}

	// Update status to success
	r.updateGenesisStatus(ctx, genesisCR, "RUNNING", "Genesis block created and stored successfully")

	log.Info("Genesis reconciliation completed successfully",
		"name", genesisCR.Name,
		"namespace", genesisCR.Namespace,
		"channelID", genesisCR.Spec.ChannelID)
	return nil
}

// validateGenesisSpec validates the Genesis spec before processing
func (r *GenesisReconciler) validateGenesisSpec(genesisCR *fabricxv1alpha1.Genesis) error {
	// Check if ChannelID is specified
	if genesisCR.Spec.ChannelID == "" {
		return fmt.Errorf("channelID is required")
	}

	// Check if at least one organization is specified
	if len(genesisCR.Spec.OrdererOrganizations) == 0 &&
		len(genesisCR.Spec.ApplicationOrgs) == 0 {
		return fmt.Errorf("at least one organization must be specified")
	}

	// Check if output configuration is specified
	if genesisCR.Spec.Output.SecretName == "" {
		return fmt.Errorf("output.secretName is required")
	}

	if genesisCR.Spec.Output.BlockKey == "" {
		return fmt.Errorf("output.blockKey is required")
	}

	// Validate organizations
	for i, org := range genesisCR.Spec.OrdererOrganizations {
		if org.Name == "" {
			return fmt.Errorf("organization %d: name is required", i)
		}
		if org.MSPID == "" {
			return fmt.Errorf("organization %s: mspID is required", org.Name)
		}
	}

	// Validate application organizations
	for i, org := range genesisCR.Spec.ApplicationOrgs {
		if org.Name == "" {
			return fmt.Errorf("application organization %d: name is required", i)
		}
		if org.MSPID == "" {
			return fmt.Errorf("application organization %s: mspID is required", org.Name)
		}
	}

	// Validate orderer nodes
	for i, node := range genesisCR.Spec.OrdererNodes {
		if node.Host == "" {
			return fmt.Errorf("orderer node %d: host is required", i)
		}
		if node.Port == 0 {
			return fmt.Errorf("orderer node %d: port is required", i)
		}
		if node.MSPID == "" {
			return fmt.Errorf("orderer node %d: mspId is required", i)
		}
	}

	return nil
}

// handleDeletion handles the deletion of a Genesis
func (r *GenesisReconciler) handleDeletion(ctx context.Context, genesisCR *fabricxv1alpha1.Genesis) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Add panic recovery
	defer func() {
		if panicErr := recover(); panicErr != nil {
			log.Error(fmt.Errorf("panic recovered: %v", panicErr), "Panic in Genesis deletion",
				"genesis", genesisCR.Name, "namespace", genesisCR.Namespace)

			// Update the Genesis status to failed
			panicMsg := fmt.Sprintf("Panic in Genesis deletion: %v", panicErr)
			r.updateGenesisStatus(ctx, genesisCR, "FAILED", panicMsg)
		}
	}()

	log.Info("Handling Genesis deletion",
		"name", genesisCR.Name,
		"namespace", genesisCR.Namespace)

	// Set status to indicate deletion
	r.updateGenesisStatus(ctx, genesisCR, "PENDING", "Deleting Genesis resources")

	// Clean up the genesis block secret if it exists
	if genesisCR.Spec.Output.SecretName != "" {
		secretName := genesisCR.Spec.Output.SecretName
		secretNamespace := genesisCR.Namespace

		log.Info("Cleaning up genesis block secret",
			"secretName", secretName,
			"secretNamespace", secretNamespace)

		// The secret will be automatically cleaned up by Kubernetes
		// when the Genesis resource is deleted, but we can log it
		log.Info("Genesis block secret will be cleaned up automatically")
	}

	// Remove finalizer
	if err := r.removeFinalizer(ctx, genesisCR); err != nil {
		errorMsg := fmt.Sprintf("Failed to remove finalizer: %v", err)
		log.Error(err, "Failed to remove finalizer")
		r.updateGenesisStatus(ctx, genesisCR, "FAILED", errorMsg)
		return ctrl.Result{}, err
	}

	log.Info("Genesis deletion completed successfully")
	return ctrl.Result{}, nil
}

// ensureFinalizer ensures the finalizer is present on the Genesis
func (r *GenesisReconciler) ensureFinalizer(ctx context.Context, genesisCR *fabricxv1alpha1.Genesis) error {
	if !utils.ContainsString(genesisCR.Finalizers, GenesisFinalizerName) {
		genesisCR.Finalizers = append(genesisCR.Finalizers, GenesisFinalizerName)
		return r.Update(ctx, genesisCR)
	}
	return nil
}

// removeFinalizer removes the finalizer from the Genesis
func (r *GenesisReconciler) removeFinalizer(ctx context.Context, genesisCR *fabricxv1alpha1.Genesis) error {
	genesisCR.Finalizers = utils.RemoveString(genesisCR.Finalizers, GenesisFinalizerName)
	return r.Update(ctx, genesisCR)
}

// updateGenesisStatus updates the Genesis status with the given status and message
func (r *GenesisReconciler) updateGenesisStatus(ctx context.Context, genesisCR *fabricxv1alpha1.Genesis, status string, message string) {
	log := logf.FromContext(ctx)

	log.Info("Updating Genesis status",
		"name", genesisCR.Name,
		"namespace", genesisCR.Namespace,
		"status", status,
		"message", message)

	// Update the status
	genesisCR.Status.Status = status
	genesisCR.Status.Message = message

	// Update the timestamp
	now := metav1.Now()
	genesisCR.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "Reconciled",
			Message:            message,
		},
	}

	// Apply the status update
	if err := r.Status().Update(ctx, genesisCR); err != nil {
		log.Error(err, "Failed to update Genesis status")
	} else {
		log.Info("Genesis status updated successfully",
			"name", genesisCR.Name,
			"namespace", genesisCR.Namespace,
			"status", status,
			"message", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *GenesisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.Genesis{}).
		Named("genesis").
		Complete(r)
}
