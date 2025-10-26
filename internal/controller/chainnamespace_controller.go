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
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
	committertypes "github.com/hyperledger/fabric-x-committer/api/types"
	"github.com/hyperledger/fabric-x-committer/utils/signature"
	"github.com/hyperledger/fabric-x-common/cmd/common/comm"
	"github.com/hyperledger/fabric-x-common/internaltools/configtxgen/encoder"
	"github.com/hyperledger/fabric-x-common/internaltools/pkg/identity"
	"github.com/hyperledger/fabric-x-common/msp"
	"github.com/hyperledger/fabric-x-common/protoutil"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

const (
	ChainNamespaceFinalizerName = "chainnamespace.fabricx.kfsoft.tech/finalizer"
)

// ChainNamespaceReconciler reconciles a Namespace object
type ChainNamespaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=chainnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=chainnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=chainnamespaces/finalizers,verbs=update
// +kubebuilder:rbac:groups=fabricx.kfsoft.tech,resources=identities,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ChainNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Namespace instance
	ns := &fabricxv1alpha1.ChainNamespace{}
	if err := r.Get(ctx, req.NamespacedName, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if the resource is being deleted
	if !ns.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, ns)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(ns, ChainNamespaceFinalizerName) {
		controllerutil.AddFinalizer(ns, ChainNamespaceFinalizerName)
		if err := r.Update(ctx, ns); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Skip if already deployed successfully
	if ns.Status.Status == "Deployed" && ns.Status.TxID != "" {
		log.Info("Namespace already deployed", "txID", ns.Status.TxID)
		return ctrl.Result{}, nil
	}

	log.Info("Deploying namespace",
		"name", ns.Spec.Name,
		"mspID", ns.Spec.MSPID,
		"orderer", ns.Spec.Orderer,
		"channel", ns.Spec.Channel)

	// Deploy the namespace
	txID, err := r.deployNamespace(ctx, ns)
	if err != nil {
		log.Error(err, "Failed to deploy namespace")
		if updateErr := r.updateStatus(ctx, ns, "Failed", err.Error(), ""); updateErr != nil {
			log.Error(updateErr, "Failed to update status after deployment failure")
		}
		return ctrl.Result{}, err
	}

	// Update status to deployed
	if err := r.updateStatus(ctx, ns, "Deployed", "Namespace deployed successfully", txID); err != nil {
		log.Error(err, "Failed to update status after successful deployment")
		return ctrl.Result{}, err
	}

	log.Info("Namespace deployed successfully", "txID", txID)
	return ctrl.Result{}, nil
}

// deployNamespace orchestrates the namespace deployment process
func (r *ChainNamespaceReconciler) deployNamespace(ctx context.Context, ns *fabricxv1alpha1.ChainNamespace) (string, error) {
	log := logf.FromContext(ctx)

	// Validate configuration
	if err := r.validateNamespace(ns); err != nil {
		return "", fmt.Errorf("validation error: %w", err)
	}

	// Create temporary directory for MSP
	tmpDir, err := os.MkdirTemp("", "namespace-msp-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	// DO NOT clean up tmpDir - keep it for debugging
	// defer os.RemoveAll(tmpDir)
	log.Info("Created temporary MSP directory (will NOT be cleaned up for debugging)", "tmpDir", tmpDir)

	// Setup MSP from Identity CRD
	thisMSP, err := r.setupMSPFromIdentity(ctx, ns.Spec.Identity, tmpDir)
	if err != nil {
		return "", fmt.Errorf("failed to setup MSP: %w", err)
	}

	// Get the signing identity
	sid, err := thisMSP.GetDefaultSigningIdentity()
	if err != nil {
		return "", fmt.Errorf("failed to get signing identity: %w", err)
	}

	// Get public key for namespace policy
	var pkData []byte
	if ns.Spec.VerificationKeyPath != "" {
		pkData, err = os.ReadFile(ns.Spec.VerificationKeyPath)
		if err != nil {
			return "", fmt.Errorf("failed to read verification key: %w", err)
		}
	} else {
		// Use the default MSP signer as namespace endorsement policy
		log.Info("Extracting public key from signing identity", "identity", sid.GetIdentifier())
		pkData, err = r.extractPublicPem(sid)
		if err != nil {
			return "", fmt.Errorf("failed to extract public key from signing identity %s: %w", sid.GetIdentifier(), err)
		}
	}

	// Get serialized public key for namespace policy
	serializedPublicKey, err := r.getPubKeyFromPemData(ctx, pkData)
	if err != nil {
		return "", fmt.Errorf("failed to get public key from pem: %w", err)
	}

	// Create namespace transaction
	tx := r.createNamespacesTx("ECDSA", serializedPublicKey, ns.Spec.Name, ns.Spec.Version)

	// Create signed envelope
	channel := ns.Spec.Channel
	if channel == "" {
		channel = "mychannel" // Default channel name
	}
	env, txID, err := r.createSignedEnvelope(sid, channel, tx)
	if err != nil {
		return "", fmt.Errorf("failed to create signed envelope: %w", err)
	}

	// Broadcast to orderer (without TLS)
	if err := r.broadcast(ctx, ns.Spec.Orderer, env); err != nil {
		return "", fmt.Errorf("failed to broadcast: %w", err)
	}

	log.Info("Namespace transaction broadcasted successfully", "txID", txID)
	return txID, nil
}

// validateNamespace validates the namespace configuration
func (r *ChainNamespaceReconciler) validateNamespace(ns *fabricxv1alpha1.ChainNamespace) error {
	if ns.Spec.Name == "" {
		return errors.New("namespace name cannot be empty")
	}
	if ns.Spec.Orderer == "" {
		return errors.New("orderer endpoint cannot be empty")
	}
	if ns.Spec.MSPID == "" {
		return errors.New("mspID cannot be empty")
	}
	if ns.Spec.Identity.Name == "" || ns.Spec.Identity.Namespace == "" {
		return errors.New("identity reference must specify name and namespace")
	}
	if ns.Spec.CACert.Name == "" || ns.Spec.CACert.Namespace == "" {
		return errors.New("caCert reference must specify name and namespace")
	}
	return nil
}

// extractPublicPem extracts the public key from a signing identity
func (r *ChainNamespaceReconciler) extractPublicPem(sid msp.SigningIdentity) ([]byte, error) {
	sidBytes, err := sid.Serialize()
	if err != nil {
		return nil, err
	}

	mspSI, err := protoutil.UnmarshalSerializedIdentity(sidBytes)
	if err != nil {
		return nil, err
	}

	return mspSI.IdBytes, nil
}

// getPubKeyFromPemData looks for ECDSA public key in pemContent and returns PEM content only with the public key
func (r *ChainNamespaceReconciler) getPubKeyFromPemData(ctx context.Context, pemContent []byte) ([]byte, error) {
	log := logf.FromContext(ctx)
	log.Info("Getting public key from pem data", "pemContent", string(pemContent))
	for {
		block, rest := pem.Decode(pemContent)
		if block == nil {
			break
		}
		pemContent = rest

		key, err := encoder.ParseCertificateOrPublicKey(block.Bytes)
		if err != nil {
			continue
		}

		return pem.EncodeToMemory(&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: key,
		}), nil
	}

	return nil, errors.New("no ECDSA public key in pem file")
}

// createNamespacesTx creates a namespace transaction
func (r *ChainNamespaceReconciler) createNamespacesTx(policyScheme string, policy []byte, nsID string, nsVersion int) *protoblocktx.Tx {
	writeToMetaNs := &protoblocktx.TxNamespace{
		NsId:       committertypes.MetaNamespaceID,
		NsVersion:  uint64(0),
		ReadWrites: make([]*protoblocktx.ReadWrite, 0, 1),
	}

	nsPolicy := &protoblocktx.NamespacePolicy{
		Scheme:    policyScheme,
		PublicKey: policy,
	}

	policyBytes := protoutil.MarshalOrPanic(nsPolicy)
	rw := &protoblocktx.ReadWrite{
		Key:   []byte(nsID),
		Value: policyBytes,
	}

	// Only set the version if we update a namespace policy
	if nsVersion >= 0 {
		rw.Version = committertypes.Version(uint64(nsVersion))
	}

	writeToMetaNs.ReadWrites = append(writeToMetaNs.ReadWrites, rw)

	tx := &protoblocktx.Tx{
		Namespaces: []*protoblocktx.TxNamespace{
			writeToMetaNs,
		},
	}

	return tx
}

// createSignedEnvelope creates a signed envelope from the transaction
func (r *ChainNamespaceReconciler) createSignedEnvelope(signer identity.SignerSerializer, channel string, tx *protoblocktx.Tx) (*cb.Envelope, string, error) {
	signatureHdr := protoutil.NewSignatureHeaderOrPanic(signer)

	// Compute transaction ID
	txID := protoutil.ComputeTxID(signatureHdr.Nonce, signatureHdr.Creator)

	// Sign each namespace
	tx.Signatures = make([][]byte, len(tx.GetNamespaces()))
	for idx, ns := range tx.GetNamespaces() {
		// Note that a default msp signer hashes the msg before signing.
		// For that reason we use the TxNamespace message as ASN1 encoded msg
		msg, err := signature.ASN1MarshalTxNamespace(txID, ns)
		if err != nil {
			return nil, "", fmt.Errorf("failed asn1 marshal tx: %w", err)
		}

		sig, err := signer.Sign(msg)
		if err != nil {
			return nil, "", fmt.Errorf("failed signing tx: %w", err)
		}
		tx.Signatures[idx] = sig
	}

	channelHdr := protoutil.MakeChannelHeader(cb.HeaderType_MESSAGE, 0, channel, 0)
	channelHdr.TxId = txID

	payloadHdr := protoutil.MakePayloadHeader(channelHdr, signatureHdr)
	txBytes := protoutil.MarshalOrPanic(tx)
	env, err := r.createEnvelope(signer, payloadHdr, txBytes)
	if err != nil {
		return nil, "", err
	}
	return env, txID, nil
}

// createEnvelope creates a signed envelope from the passed header and data
func (r *ChainNamespaceReconciler) createEnvelope(signer identity.SignerSerializer, hdr *cb.Header, data []byte) (*cb.Envelope, error) {
	payloadBytes := protoutil.MarshalOrPanic(
		&cb.Payload{
			Header: hdr,
			Data:   data,
		},
	)

	var sig []byte
	if signer != nil {
		var err error
		sig, err = signer.Sign(payloadBytes)
		if err != nil {
			return nil, err
		}
	}

	env := &cb.Envelope{
		Payload:   payloadBytes,
		Signature: sig,
	}

	return env, nil
}

// broadcast sends the envelope to the orderer (without TLS for now)
func (r *ChainNamespaceReconciler) broadcast(ctx context.Context, ordererEndpoint string, env *cb.Envelope) error {
	// Create comm client config without TLS
	config := comm.Config{}
	log := logf.FromContext(ctx)

	cl, err := comm.NewClient(config)
	if err != nil {
		return fmt.Errorf("cannot create grpc client: %w", err)
	}

	conn, err := cl.NewDialer(ordererEndpoint)()
	if err != nil {
		return fmt.Errorf("cannot dial orderer: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	occ := ab.NewAtomicBroadcastClient(conn)

	abc, err := occ.Broadcast(ctx)
	if err != nil {
		return fmt.Errorf("cannot create broadcast client: %w", err)
	}

	err = abc.Send(env)
	if err != nil {
		return fmt.Errorf("failed to send envelope: %w", err)
	}

	status, err := abc.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive response: %w", err)
	}

	if status.GetStatus() != cb.Status_SUCCESS {
		return fmt.Errorf("broadcast failed with status: %v", status.GetStatus())
	}
	log.Info("Broadcast successful", "status", status.GetStatus(), "info", status.GetInfo())

	return nil
}

// handleDeletion handles the cleanup when a Namespace resource is being deleted
func (r *ChainNamespaceReconciler) handleDeletion(ctx context.Context, ns *fabricxv1alpha1.ChainNamespace) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(ns, ChainNamespaceFinalizerName) {
		return ctrl.Result{}, nil
	}

	// No cleanup needed for namespaces - they persist in the blockchain
	log.Info("Namespace resource deleted (blockchain namespace persists)", "name", ns.Spec.Name)

	// Remove finalizer
	controllerutil.RemoveFinalizer(ns, ChainNamespaceFinalizerName)
	if err := r.Update(ctx, ns); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateStatus updates the ChainNamespace status with the given values
// Helper function that can be used when implementing the deployment logic
func (r *ChainNamespaceReconciler) updateStatus(ctx context.Context, ns *fabricxv1alpha1.ChainNamespace, status, message, txID string) error {
	ns.Status.Status = status
	ns.Status.Message = message
	ns.Status.TxID = txID

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: ns.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             status,
		Message:            message,
	}

	if status == "Failed" {
		condition.Status = metav1.ConditionFalse
	}

	// Find and update existing condition or append new one
	found := false
	for i, cond := range ns.Status.Conditions {
		if cond.Type == condition.Type {
			ns.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		ns.Status.Conditions = append(ns.Status.Conditions, condition)
	}

	return r.Status().Update(ctx, ns)
}

// setupMSPFromIdentity creates a local MSP directory structure from an Identity CRD,
// initializes the MSP, and returns the configured msp.MSP instance ready to use.
//
// The function creates the following structure:
// <tmpDir>/msp/
//
//	├── signcerts/
//	│   └── cert.pem
//	├── keystore/
//	│   └── priv_sk
//	├── cacerts/
//	│   └── ca.pem
//	└── config.yaml (NodeOUs configuration)
//
// Parameters:
//   - ctx: Context for the operation
//   - identityRef: Reference to the Identity resource (name and namespace)
//   - tmpDir: Temporary directory where to create the MSP structure
//
// Returns:
//   - msp.MSP: Initialized MSP instance ready for signing
//   - error: Any error encountered
func (r *ChainNamespaceReconciler) setupMSPFromIdentity(ctx context.Context, identityRef fabricxv1alpha1.SecretKeyRef, tmpDir string) (msp.MSP, error) {
	log := logf.FromContext(ctx)

	log.V(1).Info("Starting MSP setup from Identity",
		"identityName", identityRef.Name,
		"identityNamespace", identityRef.Namespace,
		"tmpDir", tmpDir)

	// Fetch the Identity resource
	identity := &fabricxv1alpha1.Identity{}
	log.V(1).Info("Fetching Identity resource", "name", identityRef.Name, "namespace", identityRef.Namespace)
	if err := r.Get(ctx, types.NamespacedName{
		Name:      identityRef.Name,
		Namespace: identityRef.Namespace,
	}, identity); err != nil {
		return nil, fmt.Errorf("failed to get Identity %s/%s: %w", identityRef.Namespace, identityRef.Name, err)
	}
	log.V(1).Info("Identity resource fetched successfully",
		"type", identity.Spec.Type,
		"mspID", identity.Spec.MspID,
		"status", identity.Status.Status)

	// Check if identity is ready (Status can be "Ready" or "READY")
	if identity.Status.Status != "Ready" && identity.Status.Status != "READY" {
		return nil, fmt.Errorf("identity %s/%s is not ready (status: %s)", identityRef.Namespace, identityRef.Name, identity.Status.Status)
	}

	mspID := identity.Spec.MspID
	log.Info("Setting up MSP from Identity", "identity", identityRef.Name, "mspID", mspID)

	// Use the single consolidated secret from the identity's output configuration
	secretName := identity.Spec.Output.SecretName
	outputNamespace := identityRef.Namespace

	log.Info("Using consolidated secret from identity output",
		"secretName", secretName,
		"namespace", outputNamespace)

	// Create MSP directory structure
	mspPath := filepath.Join(tmpDir, "msp")
	log.V(1).Info("Creating MSP directory structure", "mspPath", mspPath, "tmpDir", tmpDir)

	dirs := []string{
		filepath.Join(mspPath, "signcerts"),
		filepath.Join(mspPath, "keystore"),
		filepath.Join(mspPath, "cacerts"),
	}

	for _, dir := range dirs {
		log.V(1).Info("Creating directory", "path", dir, "permissions", "0755")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		log.V(1).Info("Directory created successfully", "path", dir)
	}

	// Retrieve signing certificate from consolidated secret
	log.V(1).Info("Retrieving signing certificate from secret",
		"secretName", secretName,
		"namespace", outputNamespace,
		"key", "cert.pem")
	signCertData, err := r.getSecretData(ctx, secretName, outputNamespace, "cert.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to get sign certificate: %w", err)
	}
	log.V(1).Info("Retrieved signing certificate", "sizeBytes", len(signCertData))

	// Write signing certificate
	certPath := filepath.Join(mspPath, "signcerts", "cert.pem")
	log.V(1).Info("Writing signing certificate", "path", certPath, "size", len(signCertData))
	if err := os.WriteFile(certPath, signCertData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write certificate to %s: %w", certPath, err)
	}
	log.V(1).Info("Signing certificate written successfully", "path", certPath)

	// Retrieve signing key from consolidated secret (stored as key.pem)
	log.V(1).Info("Retrieving signing key from secret",
		"secretName", secretName,
		"namespace", outputNamespace,
		"key", "key.pem")
	signKeyData, err := r.getSecretData(ctx, secretName, outputNamespace, "key.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to get sign key: %w", err)
	}
	log.V(1).Info("Retrieved signing key", "sizeBytes", len(signKeyData))

	// Write signing key
	keyPath := filepath.Join(mspPath, "keystore", "priv_sk")
	log.V(1).Info("Writing signing key", "path", keyPath, "size", len(signKeyData), "permissions", "0600")
	if err := os.WriteFile(keyPath, signKeyData, 0600); err != nil {
		return nil, fmt.Errorf("failed to write key to %s: %w", keyPath, err)
	}
	log.V(1).Info("Signing key written successfully", "path", keyPath)

	// Retrieve CA certificate from consolidated secret (stored as cacert.pem)
	log.V(1).Info("Retrieving CA certificate from secret",
		"secretName", secretName,
		"namespace", outputNamespace,
		"key", "cacert.pem")
	caCertData, err := r.getSecretData(ctx, secretName, outputNamespace, "cacert.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to get CA certificate: %w", err)
	}
	log.V(1).Info("Retrieved CA certificate", "sizeBytes", len(caCertData))

	// Write CA certificate
	caPath := filepath.Join(mspPath, "cacerts", "ca.pem")
	log.V(1).Info("Writing CA certificate", "path", caPath, "size", len(caCertData))
	if err := os.WriteFile(caPath, caCertData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write CA cert to %s: %w", caPath, err)
	}
	log.V(1).Info("CA certificate written successfully", "path", caPath)

	// Create config.yaml for NodeOUs
	configContent := `NodeOUs:
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

	configPath := filepath.Join(mspPath, "config.yaml")
	log.V(1).Info("Writing NodeOU config.yaml", "path", configPath, "size", len(configContent))
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write config.yaml to %s: %w", configPath, err)
	}
	log.V(1).Info("NodeOU config.yaml written successfully", "path", configPath)

	// List final directory structure for debugging
	log.V(1).Info("MSP directory structure created", "mspPath", mspPath)
	log.V(1).Info("MSP contents:",
		"signcerts", certPath,
		"keystore", keyPath,
		"cacerts", caPath,
		"config", configPath)

	log.Info("Successfully created MSP directory structure", "mspPath", mspPath, "mspID", mspID)

	// Initialize the MSP
	thisMSP, err := r.initializeMSP(mspPath, mspID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MSP: %w", err)
	}

	log.Info("Successfully initialized MSP", "mspID", mspID)
	return thisMSP, nil
}

// initializeMSP sets up and returns an MSP instance from the given MSP directory path
// This mirrors the setupMSP function from fabric-x-common/cmd/common/namespace
func (r *ChainNamespaceReconciler) initializeMSP(mspConfigPath, mspID string) (msp.MSP, error) {
	// Get the MSP configuration
	conf, err := msp.GetLocalMspConfig(mspConfigPath, nil, mspID)
	if err != nil {
		return nil, fmt.Errorf("error getting local msp config from %v: %w", mspConfigPath, err)
	}

	// Setup keystore
	dir := path.Join(mspConfigPath, "keystore")
	ks, err := sw.NewFileBasedKeyStore(nil, dir, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create keystore: %w", err)
	}

	// Setup crypto provider
	cp, err := sw.NewDefaultSecurityLevelWithKeystore(ks)
	if err != nil {
		return nil, fmt.Errorf("failed to create crypto provider: %w", err)
	}

	// Create MSP options
	mspOpts := &msp.BCCSPNewOpts{
		NewBaseOpts: msp.NewBaseOpts{
			Version: msp.MSPv1_0,
		},
	}

	// Create and setup MSP
	thisMSP, err := msp.New(mspOpts, cp)
	if err != nil {
		return nil, fmt.Errorf("failed to create MSP: %w", err)
	}

	err = thisMSP.Setup(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to setup MSP: %w", err)
	}

	return thisMSP, nil
}

// getSecretData retrieves data from a Kubernetes secret by secret name and key
func (r *ChainNamespaceReconciler) getSecretData(ctx context.Context, secretName, namespace, key string) ([]byte, error) {
	if secretName == "" {
		return nil, fmt.Errorf("secret name is empty")
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretName, err)
	}

	data, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in secret %s/%s", key, namespace, secretName)
	}

	return data, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChainNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricxv1alpha1.ChainNamespace{}).
		Complete(r)
}
