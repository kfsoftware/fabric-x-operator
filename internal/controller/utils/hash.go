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

package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ComputeSecretHash computes a SHA256 hash of all data in a Kubernetes Secret.
// The hash is deterministic - it will produce the same result for the same secret data.
// Returns the hex-encoded hash string, or an error if the secret cannot be fetched.
func ComputeSecretHash(ctx context.Context, k8sClient client.Client, secretName, namespace string) (string, error) {
	sec := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, sec); err != nil {
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

// ComputeConfigMapHash computes a SHA256 hash of all data in a Kubernetes ConfigMap.
// The hash is deterministic - it will produce the same result for the same configmap data.
// Returns the hex-encoded hash string, or an error if the configmap cannot be fetched.
func ComputeConfigMapHash(ctx context.Context, k8sClient client.Client, configMapName, namespace string) (string, error) {
	cm := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: namespace}, cm); err != nil {
		return "", err
	}

	var parts []string
	for k, v := range cm.Data {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range cm.BinaryData {
		parts = append(parts, fmt.Sprintf("%s=%s", k, string(v)))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:]), nil
}

// ComputeEnvVarsHash computes a SHA256 hash of environment variables.
// The hash is deterministic - it will produce the same result for the same env vars.
// Returns the hex-encoded hash string.
func ComputeEnvVarsHash(envVars []corev1.EnvVar) string {
	if len(envVars) == 0 {
		return ""
	}

	envString := ""
	for _, env := range envVars {
		envString += fmt.Sprintf("%s=%s|", env.Name, env.Value)
	}
	envHashSum := sha256.Sum256([]byte(envString))
	return hex.EncodeToString(envHashSum[:])
}

// ComputeCombinedHash combines multiple hash strings into a single deterministic hash.
// The input hashes are sorted before combining to ensure deterministic results.
// Returns the hex-encoded combined hash string.
func ComputeCombinedHash(hashes []string) string {
	sort.Strings(hashes)
	combinedHashSum := sha256.Sum256([]byte(strings.Join(hashes, "|")))
	return hex.EncodeToString(combinedHashSum[:])
}

// HashBuilder is a helper for building combined hashes from multiple sources.
type HashBuilder struct {
	hashes []string
}

// NewHashBuilder creates a new HashBuilder.
func NewHashBuilder() *HashBuilder {
	return &HashBuilder{
		hashes: make([]string, 0),
	}
}

// AddSecret adds a secret hash to the builder.
// If the secret cannot be fetched, it logs a warning but continues.
func (hb *HashBuilder) AddSecret(ctx context.Context, k8sClient client.Client, secretName, namespace string) *HashBuilder {
	if hash, err := ComputeSecretHash(ctx, k8sClient, secretName, namespace); err == nil {
		hb.hashes = append(hb.hashes, hash)
	}
	return hb
}

// AddConfigMap adds a configmap hash to the builder.
// If the configmap cannot be fetched, it logs a warning but continues.
func (hb *HashBuilder) AddConfigMap(ctx context.Context, k8sClient client.Client, configMapName, namespace string) *HashBuilder {
	if hash, err := ComputeConfigMapHash(ctx, k8sClient, configMapName, namespace); err == nil {
		hb.hashes = append(hb.hashes, hash)
	}
	return hb
}

// AddEnvVars adds environment variables hash to the builder.
func (hb *HashBuilder) AddEnvVars(envVars []corev1.EnvVar) *HashBuilder {
	if hash := ComputeEnvVarsHash(envVars); hash != "" {
		hb.hashes = append(hb.hashes, hash)
	}
	return hb
}

// AddString adds a direct string hash to the builder.
func (hb *HashBuilder) AddString(value string) *HashBuilder {
	if value != "" {
		hash := sha256.Sum256([]byte(value))
		hb.hashes = append(hb.hashes, hex.EncodeToString(hash[:]))
	}
	return hb
}

// Build returns the combined hash of all added sources.
func (hb *HashBuilder) Build() string {
	return ComputeCombinedHash(hb.hashes)
}

// UpdateConditionIfChanged updates a condition only if it has actually changed.
// Returns true if the condition was updated, false otherwise.
// This prevents unnecessary status updates that trigger reconciliation loops.
func UpdateConditionIfChanged(
	existingConditions []metav1.Condition,
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
	message string,
) ([]metav1.Condition, bool) {
	now := metav1.Now()
	var lastTransitionTime metav1.Time
	conditionChanged := false

	// Find existing condition
	var foundIndex = -1
	for i, cond := range existingConditions {
		if cond.Type == conditionType {
			foundIndex = i
			// Check if anything actually changed
			if cond.Status != status || cond.Reason != reason || cond.Message != message {
				conditionChanged = true
				lastTransitionTime = now
			} else {
				// Nothing changed, keep the same lastTransitionTime
				lastTransitionTime = cond.LastTransitionTime
			}
			break
		}
	}

	// If condition not found, it's a new condition
	if foundIndex == -1 {
		conditionChanged = true
		lastTransitionTime = now
	}

	newCondition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: lastTransitionTime,
		Reason:             reason,
		Message:            message,
	}

	// Build new conditions list
	newConditions := make([]metav1.Condition, 0, len(existingConditions))
	if foundIndex >= 0 {
		// Replace existing condition
		for i, cond := range existingConditions {
			if i == foundIndex {
				newConditions = append(newConditions, newCondition)
			} else {
				newConditions = append(newConditions, cond)
			}
		}
	} else {
		// Append new condition
		newConditions = append(newConditions, existingConditions...)
		newConditions = append(newConditions, newCondition)
	}

	return newConditions, conditionChanged
}
