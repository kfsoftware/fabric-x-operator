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
	"fmt"
	"strings"
	"text/template"
)

// TemplateData represents common data used in templates
type TemplateData struct {
	Name        string
	PartyID     int32
	MSPID       string
	ShardID     int32
	ConsenterID int32
	Port        int32
	Instance    int32
}

// BatcherTemplateData represents data specific to batcher templates
type BatcherTemplateData struct {
	Name    string
	PartyID int32
	MSPID   string
	ShardID int32
	Port    int32
}

// RouterTemplateData represents data specific to router templates
type RouterTemplateData struct {
	Name    string
	PartyID int32
	MSPID   string
	Port    int32
}

// ConsenterTemplateData represents data specific to consenter templates
type ConsenterTemplateData struct {
	Name        string
	PartyID     int32
	MSPID       string
	ConsenterID int32
	Port        int32
}

// AssemblerTemplateData represents data specific to assembler templates
type AssemblerTemplateData struct {
	Name    string
	PartyID int32
	MSPID   string
	Port    int32
}

// ExecuteTemplate executes a Go template with the given data
func ExecuteTemplate(templateStr string, data interface{}) (string, error) {
	// Parse the template
	tmpl, err := template.New("config").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute the template
	var buffer strings.Builder
	if err := tmpl.Execute(&buffer, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buffer.String(), nil
}

// ExecuteTemplateWithValidation executes a template and validates the result
func ExecuteTemplateWithValidation(templateStr string, data interface{}) (string, error) {
	result, err := ExecuteTemplate(templateStr, data)
	if err != nil {
		return "", err
	}

	// Basic validation - ensure the result is not empty
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("template execution resulted in empty output")
	}

	return result, nil
}

// Common template constants that can be reused across controllers
const (
	// BatcherConfigTemplate is the Go template for batcher configuration
	BatcherConfigTemplate = `PartyID: {{.PartyID}}
General:
    ListenAddress: 0.0.0.0
    ListenPort: {{.Port}}
    TLS:
        Enabled: false
        PrivateKey: /{{.Name}}/tls/server.key
        Certificate: /{{.Name}}/tls/server.crt
        RootCAs:
            - /{{.Name}}/tls/ca.crt
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
        File: /{{.Name}}/genesis.block
    LocalMSPDir: /{{.Name}}/msp
    LocalMSPID: {{.MSPID}}
    LogSpec: info
FileStore:
    Location: /{{.Name}}/store
Batcher:
    ShardID: {{.ShardID}}
    BatchSequenceGap: 12
    MemPoolMaxSize: 1200000
    SubmitTimeout: 600ms`

	// RouterConfigTemplate is the Go template for router configuration
	RouterConfigTemplate = `PartyID: {{.PartyID}}
General:
    ListenAddress: 0.0.0.0
    ListenPort: {{.Port}}
    TLS:
        Enabled: false
        PrivateKey: /{{.Name}}/tls/server.key
        Certificate: /{{.Name}}/tls/server.crt
        RootCAs:
            - /{{.Name}}/tls/ca.crt
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
        File: /{{.Name}}/genesis.block
    LocalMSPDir: /{{.Name}}/msp
    LocalMSPID: {{.MSPID}}
    LogSpec: info
FileStore:
    Location: /{{.Name}}/store
Router:
    RoutingTable:
        UpdateInterval: 30s
        MaxRoutes: 1000
    LoadBalancing:
        Algorithm: round_robin
        HealthCheckInterval: 10s
    CircuitBreaker:
        Enabled: true
        FailureThreshold: 5
        RecoveryTimeout: 30s
`

	// ConsenterConfigTemplate is the Go template for consenter configuration
	ConsenterConfigTemplate = `PartyID: {{.PartyID}}
ConsenterID: {{.ConsenterID}}
General:
    ListenAddress: 0.0.0.0
    ListenPort: {{.Port}}
    TLS:
        Enabled: false
        PrivateKey: ./tls/server.key
        Certificate: ./tls/server.crt
        RootCAs:
            - ./tls/ca.crt
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
        File: ./genesis.block
    Cluster:
        SendBufferSize: 2000
        ClientCertificate: ./tls/server.crt
        ClientPrivateKey: ./tls/server.key
    LocalMSPDir: ./msp
    LocalMSPID: {{.MSPID}}
    LogSpec: info
FileStore:
    Location: ./store
Consensus:
    WALDir: ./wal
    ConsensusType: pbft
    BatchTimeout: 2s
    BatchSize:
        MaxMessageCount: 500
        AbsoluteMaxBytes: 10MB
        PreferredMaxBytes: 2MB
`

	// AssemblerConfigTemplate is the Go template for assembler configuration
	AssemblerConfigTemplate = `PartyID: {{.PartyID}}
General:
    ListenAddress: 0.0.0.0
    ListenPort: {{.Port}}
    TLS:
        Enabled: false
        PrivateKey: /{{.Name}}/tls/server.key
        Certificate: /{{.Name}}/tls/server.crt
        RootCAs:
            - /{{.Name}}/tls/ca.crt
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
        File: /{{.Name}}/genesis.block
    LocalMSPDir: /{{.Name}}/msp
    LocalMSPID: {{.MSPID}}
    LogSpec: info
FileStore:
    Location: /{{.Name}}/store
Assembler:
    BatchTimeout: 2s
    BatchSize:
        MaxMessageCount: 500
        AbsoluteMaxBytes: 10MB
        PreferredMaxBytes: 2MB
    AssemblyRules:
        MaxAssemblyTime: 5s
        MinBatchSize: 10
        MaxBatchSize: 1000
`
)
