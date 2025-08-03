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