# Act Quick Reference

Quick reference for testing GitHub Actions locally with `act`.

## Installation

```bash
# macOS
brew install act

# Linux
curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash
```

## Makefile Targets

```bash
# List all workflows
make act-list

# Test lint workflow (dry run)
make act-lint

# Test unit test workflow (dry run)
make act-test

# Test e2e workflow (dry run - validates syntax only)
make act-e2e
```

## Direct Commands

### List Workflows
```bash
act -l
```

### Dry Run (Validate Syntax)
```bash
# Test specific job
act -j lint -n
act -j test -n
act -j test-e2e-k3d -n

# Test all workflows
act push -n
```

### Run Workflow (Actual Execution)
```bash
# WARNING: e2e tests may not work due to Docker-in-Docker limitations
# Use for lint and test workflows only

# Run lint workflow
act -j lint push

# Run test workflow
act -j test push
```

## Common Options

```bash
# Verbose output
act -j lint -v

# Very verbose (debug)
act -j lint -v -v

# List available jobs
act -l

# Dry run (no execution)
act -j lint -n

# Keep container running after failure
act -j lint --rm=false

# Use different event
act pull_request
```

## Workflow Testing Strategy

### ✅ Works Well with Act
- **Lint workflow**: Syntax validation, quick
- **Unit test workflow**: No external dependencies
- **Syntax validation**: All workflows (with `-n` flag)

### ⚠️ Limited Support
- **E2E workflows**: Docker-in-Docker limitations
  - Use `act -j test-e2e-k3d -n` for syntax validation only
  - Use `make test-e2e-k3d` for actual testing

## Quick Workflow Validation

Before pushing changes:

```bash
# 1. List workflows to ensure they're detected
make act-list

# 2. Validate lint workflow
make act-lint

# 3. Validate test workflow
make act-test

# 4. Validate e2e workflow syntax
make act-e2e

# 5. Run actual e2e tests locally (not through act)
make test-e2e-k3d
```

## Configuration

Project includes `.actrc` with optimal settings:
- Uses `catthehacker/ubuntu:act-latest` image
- Configures artifact server
- Enables host network mode

## Troubleshooting

### Docker Socket Error
```bash
# Ensure Docker is running
docker ps

# Check permissions
ls -la /var/run/docker.sock
```

### Image Pull Issues
```bash
# Use cached images
act --pull=false -j lint

# Manually pull image
docker pull catthehacker/ubuntu:act-latest
```

### Workflow Not Found
```bash
# Check workflow files
ls -la .github/workflows/

# Verify act can detect them
act -l
```

## Resources

- Full documentation: [docs/testing-github-actions-locally.md](testing-github-actions-locally.md)
- Act repository: https://github.com/nektos/act
- GitHub Actions syntax: https://docs.github.com/en/actions
