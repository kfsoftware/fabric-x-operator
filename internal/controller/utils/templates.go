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
	DataDir     string
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

// CommitterCoordinatorTemplateData represents data specific to coordinator templates
type CommitterCoordinatorTemplateData struct {
	Name                        string
	PartyID                     int32
	MSPID                       string
	Port                        int32
	VerifierEndpoints           []string
	ValidatorCommitterEndpoints []string
}

// CommitterQueryServiceTemplateData holds configuration data for CommitterQueryService
type CommitterQueryServiceTemplateData struct {
	Name       string
	PartyID    int32
	MSPID      string
	Port       int32
	PostgreSQL *PostgreSQLTemplateData
}

// BuildQueryServiceConfig returns a Query Service config template
func BuildQueryServiceConfig(data CommitterQueryServiceTemplateData) string {
	config := fmt.Sprintf(`server:
  # The server's endpoint configuration
  endpoint: 0.0.0.0:%d

# Resource limit configurations
# A batch will execute once it accumulated this number of keys.
min-batch-keys: 1024
# A batch will execute once it waited this much time.
max-batch-wait: 100ms
# A new view will be created if the previous view was created before this much time.
view-aggregation-window: 100ms
# A new view will be created if the previous view aggregated this number of views.
max-aggregated-views: 1024
# A view will be closed if it was opened for longer than this time.
max-view-timeout: 10s
`, data.Port)

	// Add database configuration if provided
	if data.PostgreSQL != nil {
		config += fmt.Sprintf(`
database:
  endpoints:
    - %s:%d
  username: %s
  password: %s
  database: %s
  max-connections: %d
  min-connections: %d
  load-balance: %t
  retry:
    max-elapsed-time: %s
`, data.PostgreSQL.Host, data.PostgreSQL.Port, data.PostgreSQL.Username,
			data.PostgreSQL.Password, data.PostgreSQL.Database,
			data.PostgreSQL.MaxConnections, data.PostgreSQL.MinConnections,
			data.PostgreSQL.LoadBalance, data.PostgreSQL.MaxElapsedTime)
	}

	config += fmt.Sprintf(`
party-id: %d
msp-id: %s
`, data.PartyID, data.MSPID)

	return config
}

// BuildCoordinatorConfig returns a Coordinator config template with provided endpoints
func BuildCoordinatorConfig(data CommitterCoordinatorTemplateData, verifier []string, validatorCommitter []string) string {
	v := "\n"
	for _, e := range verifier {
		v += "    - " + e + "\n"
	}
	vc := "\n"
	for _, e := range validatorCommitter {
		vc += "    - " + e + "\n"
	}
	return `server:
  endpoint:
    host: 0.0.0.0
    port: {{.Port}}
verifier:
  endpoints:` + v + `
validator-committer:
  endpoints:` + vc + `
dependency-graph:
  num-of-local-dep-constructors: 4
  waiting-txs-limit: 20000000
  num-of-workers-for-global-dep-manager: 1
per-channel-buffer-size-per-goroutine: 10
monitoring:
  prometheus:
    enabled: true
    port: 2120
logging:
  level: INFO
  format: json`
}

// CommitterSidecarTemplateData represents data specific to sidecar templates
type CommitterSidecarTemplateData struct {
	Name             string
	PartyID          int32
	MSPID            string
	Port             int32
	OrdererEndpoints []string
	CommitterHost    string
	CommitterPort    int32
}

// CommitterValidatorTemplateData represents data specific to validator templates
type CommitterValidatorTemplateData struct {
	Name       string
	PartyID    int32
	MSPID      string
	Port       int32
	PostgreSQL *PostgreSQLTemplateData
}

// PostgreSQLTemplateData represents PostgreSQL configuration data
type PostgreSQLTemplateData struct {
	Host           string
	Port           int32
	Password       string
	Database       string
	Username       string
	MaxConnections int32
	MinConnections int32
	LoadBalance    bool
	MaxElapsedTime string
}

// CommitterVerifierTemplateData represents data specific to verifier templates
type CommitterVerifierTemplateData struct {
	Name    string
	PartyID int32
	MSPID   string
	Port    int32
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
        PrivateKey: /etc/hyperledger/fabricx/router/tls/server.key
        Certificate: /etc/hyperledger/fabricx/router/tls/server.crt
        RootCAs:
            - /etc/hyperledger/fabricx/router/tls/ca.crt
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
        File: /etc/hyperledger/fabricx/router/genesis/genesis.block
    LocalMSPDir: /etc/hyperledger/fabricx/router/msp
    LocalMSPID: {{.MSPID}}
    LogSpec: info
FileStore:
Router:
    NumberOfConnectionsPerBatcher: 12
    NumberOfStreamsPerConnection: 6
`

	// ConsenterConfigTemplate is the Go template for consenter configuration
	ConsenterConfigTemplate = `PartyID: {{.PartyID}}
ConsenterID: {{.ConsenterID}}
General:
    ListenAddress: 0.0.0.0
    ListenPort: {{.Port}}
    TLS:
        Enabled: false
        PrivateKey: /etc/hyperledger/fabricx/consenter/tls/server.key
        Certificate: /etc/hyperledger/fabricx/consenter/tls/server.crt
        RootCAs:
            - /etc/hyperledger/fabricx/consenter/tls/ca.crt
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
        File: /etc/hyperledger/fabricx/consenter/genesis/genesis.block
    Cluster:
        SendBufferSize: 2000
        ClientCertificate: /etc/hyperledger/fabricx/consenter/tls/server.crt
        ClientPrivateKey: /etc/hyperledger/fabricx/consenter/tls/server.key
    LocalMSPDir: /etc/hyperledger/fabricx/consenter/msp
    LocalMSPID: {{.MSPID}}
    LogSpec: info
FileStore:
    Location: {{.DataDir}}/store
Consensus:
    WALDir: {{.DataDir}}/wal
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
        Enabled: true
        PrivateKey: /etc/hyperledger/fabricx/assembler/tls/server.key
        Certificate: /etc/hyperledger/fabricx/assembler/tls/server.crt
        RootCAs:
            - /etc/hyperledger/fabricx/assembler/tls/ca.crt
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
        File: /etc/hyperledger/fabricx/assembler/genesis/genesis.block
    LocalMSPDir: /etc/hyperledger/fabricx/assembler/msp
    LocalMSPID: {{.MSPID}}
    LogSpec: info
FileStore:
    Location: /etc/hyperledger/fabricx/assembler/store
Assembler:
    PrefetchBufferMemoryBytes: 1342177280
    RestartLedgerScanTimeout: 6s
    PrefetchEvictionTtl: 1h30m0s
    ReplicationChannelSize: 120
    BatchRequestsChannelSize: 1200
`

	// CoordinatorConfigTemplate is the Go template for coordinator configuration
	CoordinatorConfigTemplate = `server:
  endpoint:
    host: 0.0.0.0
    port: {{.Port}}
verifier:
  endpoints:
{{- range .VerifierEndpoints }}
    - {{ . }}
{{- end }}

validator-committer:
  endpoints:
{{- range .ValidatorCommitterEndpoints }}
    - {{ . }}
{{- end }}

dependency-graph:
  num-of-local-dep-constructors: 4
  waiting-txs-limit: 20000000
  num-of-workers-for-global-dep-manager: 1
per-channel-buffer-size-per-goroutine: 10
monitoring:
  prometheus:
    enabled: true
    port: 2120
logging:
  level: INFO
  format: json`

	// SidecarConfigTemplate is the Go template for sidecar configuration
	SidecarConfigTemplate = `server:
  endpoint: 0.0.0.0:{{.Port}}
  keep-alive:
    params:
      time: 300s
      timeout: 600s
    enforcement-policy:
      min-time: 60s
      permit-without-stream: false
orderer:
  channel-id: arma
  consensus-type: BFT
  connection:
    endpoints:
{{- range .OrdererEndpoints }}
      - broadcast,deliver,{{ . }}
{{- end }}

committer:
  endpoint:
    host: {{ .CommitterHost }}
    port: {{ .CommitterPort }}
ledger: 
  path: /var/hyperledger/fabricx/ledger
last-committed-block-set-interval: 5s
bootstrap:
  genesis-block-file-path: 
monitoring:
  prometheus:
    enabled: true
    port: 2111
logging:
  level: INFO
  format: json`

	// ValidatorConfigTemplate is the Go template for validator configuration
	ValidatorConfigTemplate = `server:
  endpoint:
    host: 0.0.0.0
    port: {{.Port}}
database:
  endpoints:
    - {{.PostgreSQL.Host}}:{{.PostgreSQL.Port}}

  username: {{.PostgreSQL.Username}}
  password: {{.PostgreSQL.Password}}
  database: {{.PostgreSQL.Database}}
  max-connections: {{.PostgreSQL.MaxConnections}}
  min-connections: {{.PostgreSQL.MinConnections}}
  load-balance: {{.PostgreSQL.LoadBalance}}
  retry:
    max-elapsed-time: {{.PostgreSQL.MaxElapsedTime}}
resource-limits:
  max-workers-for-preparer: 2
  max-workers-for-validator: 2
  max-workers-for-committer: 20
  min-transaction-batch-size: 1000
monitoring:
  prometheus:
    enabled: true
    port: 2116
logging:
  level: INFO
  format: json`

	// VerifierConfigTemplate is the Go template for verifier configuration
	VerifierConfigTemplate = `server:
  endpoint: 0.0.0.0:{{.Port}}
parallel-executor:
  batch-size-cutoff: 500
  batch-time-cutoff: 2ms
  channel-buffer-size: 1000
  parallelism: 80
monitoring:
  prometheus:
    enabled: true
    port: 2115
logging:
  level: INFO
  format: json`
)
