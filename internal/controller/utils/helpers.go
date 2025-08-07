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

import "fmt"

// ContainsString checks if a slice contains a specific string
func ContainsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

// RemoveString removes a string from a slice
func RemoveString(slice []string, str string) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if item != str {
			result = append(result, item)
		}
	}
	return result
}

// Service naming utilities for consistent service naming across controllers

// GetServiceName returns the service name for a component
// This function centralizes service naming logic to ensure consistency
func GetServiceName(componentName string) string {
	return componentName
}

// GetServiceFQDN returns the fully qualified domain name for a service
func GetServiceFQDN(componentName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", GetServiceName(componentName), namespace)
}

// GetServiceNameWithSuffix returns the service name with a suffix (for backward compatibility)
// This is used by components that historically used a suffix in their service names
func GetServiceNameWithSuffix(componentName, suffix string) string {
	return fmt.Sprintf("%s-%s", componentName, suffix)
}

// GetServiceFQDNWithSuffix returns the fully qualified domain name for a service with suffix
func GetServiceFQDNWithSuffix(componentName, suffix, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", GetServiceNameWithSuffix(componentName, suffix), namespace)
}
