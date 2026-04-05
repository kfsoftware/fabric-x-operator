# kubectl-fabricx

A kubectl plugin for managing Fabric-X operator CRDs.

## Installation

### Build from source

```bash
cd kubectl-fabricx
go build -o kubectl-fabricx
```

### Install as kubectl plugin

```bash
# Copy the binary to a directory in your PATH
cp kubectl-fabricx ~/.local/bin/

# Or install it as a kubectl plugin
cp kubectl-fabricx ~/.local/bin/kubectl-fabricx
```

## Usage

### Testnet Commands

#### Create a Testnet

The `testnet create` command deploys a complete Fabric-X test network including orderer groups, genesis block, committer, and optionally a fabric namespace.

```bash
# Create a testnet with default settings (4 parties, channel 'arma')
kubectl fabricx testnet create --namespace default --channel arma

# Create a testnet with 6 orderer parties
kubectl fabricx testnet create --namespace default --channel mychannel --num-parties 6

# Create a testnet and also create a fabric namespace
kubectl fabricx testnet create --namespace default --channel arma --create-namespace --namespace-name token

# Create without waiting for resources (faster, but doesn't verify)
kubectl fabricx testnet create --namespace default --channel arma --skip-wait
```

**Options:**
- `--namespace`: Kubernetes namespace for the testnet (required, default: default)
- `--channel`: Fabric channel name (required, default: arma)
- `--num-parties`: Number of orderer parties (default: 4, range: 1-10)
- `--wait-timeout`: Timeout for waiting for resources (default: 5m)
- `--skip-wait`: Skip waiting for resources to be ready (faster but no verification)
- `--create-namespace`: Create a fabric namespace after deployment
- `--namespace-name`: Name of the fabric namespace to create (default: token)

**What gets deployed:**

1. **Orderer Groups (configure mode)**: Creates N orderer groups (parties) with certificate enrollment
2. **Genesis Block**: Creates the channel genesis block
3. **Orderer Groups (deploy mode)**: Patches orderer groups to deploy mode, starting all orderer components (router, batchers, consenter, assembler)
4. **Committer**: Deploys all committer components (coordinator, sidecar, query service, validator, verifier)
5. **Namespace (optional)**: Creates a fabric namespace if --create-namespace flag is used

**Prerequisites:**
- A running Kubernetes cluster with kubectl configured
- Fabric-X operator installed and running
- test-ca2 CA deployed in the target namespace (or update templates to use your CA)
- PostgreSQL database for committer validator (if not available, committer will fail)

**Architecture:**

The command follows the [quick-start guide](../docs/quick-start.md) workflow with extensive progress feedback:

```
━━━ Step 1: Deploying Orderer Groups (Configure Mode) ━━━
ℹ Creating OrdererGroup for party 1...
✓ Created OrdererGroup/orderergroup-party1
...
ℹ Waiting for all orderer groups to complete certificate enrollment...
✓ OrdererGroup party1 is ready

━━━ Step 2: Creating Genesis Block ━━━
ℹ Creating Genesis block for channel 'arma'...
✓ Created Secret/meta-namespace-ca
✓ Created Genesis/fabricx-genesis
✓ Genesis block is ready

━━━ Step 3: Patching Orderer Groups to Deploy Mode ━━━
ℹ Patching OrdererGroup party1 to deploy mode...
✓ OrdererGroup party1 patched to deploy mode
...

━━━ Step 4: Deploying Committer ━━━
ℹ Creating Committer...
✓ Created Committer/fabric-x-committer

━━━ Step 5: Creating Fabric Namespace ━━━
ℹ Creating Fabric namespace 'token'...
✓ Created ChainNamespace/ns-token

━━━ Deployment Complete! ━━━
✓ Testnet deployment completed successfully
```

### CA Commands

#### Create a CA

```bash
kubectl fabricx ca create --name my-ca --namespace my-namespace
```

Options:
- `--name`: Name of the CA (required)
- `--namespace`: Namespace for the CA (default: default)
- `--image`: CA image (default: hyperledger/fabric-ca:1.4.3)
- `--version`: CA version (default: 1.4.3)
- `--storage-class`: Storage class for PVC
- `--storage-size`: Storage size for PVC (default: 1Gi)
- `--db-type`: Database type (default: sqlite3)
- `--db-source`: Database source (default: /var/hyperledger/fabric-ca/fabric-ca-server.db)
- `--service-type`: Service type (default: ClusterIP)
- `--hosts`: Host names for the CA
- `--enroll-id`: Enrollment ID (default: admin)
- `--enroll-secret`: Enrollment secret (default: adminpw)
- `--output`: Output YAML instead of applying

#### Delete a CA

```bash
kubectl fabricx ca delete --name my-ca --namespace my-namespace
```

#### Enroll with a CA

```bash
kubectl fabricx ca enroll --name my-ca --namespace my-namespace
```

#### Register with a CA

```bash
kubectl fabricx ca register --name my-ca --namespace my-namespace
```

#### Revoke a certificate

```bash
kubectl fabricx ca revoke --name my-ca --namespace my-namespace
```

## Development

### Adding new commands

1. Create a new directory under `cmd/` for your command
2. Create a main command file (e.g., `mycommand.go`)
3. Create individual command files (e.g., `create.go`, `delete.go`)
4. Add the command to the main command in `cmd/kubectl-fabricx.go`

Example structure:
```
cmd/
├── ca/
│   ├── ca.go
│   ├── create.go
│   ├── delete.go
│   ├── enroll.go
│   ├── register.go
│   └── revoke.go
└── mycommand/
    ├── mycommand.go
    ├── create.go
    └── delete.go
```

### Building

```bash
go build -o kubectl-fabricx
```

### Testing

```bash
go test ./...
``` 