# Testing GitHub Actions Locally with Act

## Overview

[Act](https://github.com/nektos/act) allows you to run GitHub Actions locally using Docker. This is invaluable for:
- Testing workflow changes before pushing
- Debugging workflow failures
- Faster iteration on CI/CD changes
- Running workflows without consuming GitHub Actions minutes

## Installation

### macOS
```bash
brew install act
```

### Linux
```bash
curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash
```

### Windows
```bash
choco install act-cli
```

## Quick Start

### List Available Workflows
```bash
# List all workflows
act -l

# Example output:
# Stage  Job ID           Job name         Workflow name  Workflow file      Events
# 0      test-e2e-kind    Run with Kind    E2E Tests      test-e2e.yml       push,pull_request
# 0      test-e2e-k3d     Run with K3D     E2E Tests      test-e2e.yml       push,pull_request
```

### Run All Jobs
```bash
# Run all jobs for push event
act push

# Run all jobs for pull_request event
act pull_request
```

### Run Specific Job
```bash
# Run only the K3D e2e test job
act -j test-e2e-k3d

# Run only the Kind e2e test job
act -j test-e2e-kind
```

### Dry Run (No Execution)
```bash
# See what would run without actually running
act -n
act -n -j test-e2e-k3d
```

## Configuration

### Act Configuration File

The repository includes `.actrc` with optimal settings:

```bash
# Use medium-sized Docker images
-P ubuntu-latest=catthehacker/ubuntu:act-latest

# Enable verbose output
-v

# Bind Docker socket for Docker-in-Docker
--container-daemon-socket -

# Set artifact server path
--artifact-server-path /tmp/artifacts
```

### Custom Configuration

You can override with command-line flags:

```bash
# Use a different runner image
act -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:full-latest

# Run with secrets
act -s GITHUB_TOKEN=your-token

# Run with environment variables
act --env MY_VAR=value
```

## Testing E2E Workflows

### Prerequisites

Since our e2e tests create Kubernetes clusters, you need:
1. Docker running
2. Enough resources (4GB+ RAM for Docker)
3. K3D or Kind installed locally

### Running the K3D E2E Test

```bash
# Run the K3D e2e test job
act -j test-e2e-k3d push

# With verbose output
act -j test-e2e-k3d push -v

# Dry run to see steps
act -j test-e2e-k3d push -n
```

**Note**: This will:
1. Pull Docker image (~2GB on first run)
2. Install K3D inside the container
3. Create a K3D cluster
4. Run e2e tests
5. Clean up

**Duration**: First run ~5-10 minutes, subsequent runs ~3-5 minutes

### Running the Kind E2E Test

```bash
# Run the Kind e2e test job
act -j test-e2e-kind push
```

## Common Issues and Solutions

### Issue 1: Docker Socket Permission Denied

**Error:**
```
Cannot connect to the Docker daemon at unix:///var/run/docker.sock
```

**Solution:**
```bash
# Add your user to docker group
sudo usermod -aG docker $USER
newgrp docker

# Or run with sudo (not recommended)
sudo act -j test-e2e-k3d
```

### Issue 2: Large Docker Images

**Problem**: Act downloads large runner images (~2GB)

**Solution**: Use smaller images for non-e2e workflows:

```bash
# Use micro image for simple jobs
act -P ubuntu-latest=node:16-buster-slim

# Or use act's cached images
act --pull=false
```

### Issue 3: K3D Cluster Creation Fails

**Error:**
```
Failed to create k3d cluster
```

**Solution**: This happens because Docker-in-Docker has limitations. Options:

1. **Run tests outside of act** (recommended for e2e):
   ```bash
   # Just run locally instead of through act
   make test-e2e-k3d
   ```

2. **Use act for non-cluster workflows**:
   ```bash
   # Test the lint workflow instead
   act -j lint
   ```

3. **Use privileged mode** (security risk):
   ```bash
   act -j test-e2e-k3d --privileged
   ```

### Issue 4: Network Issues in Container

**Error:**
```
Cannot reach external services
```

**Solution**:
```bash
# Use host network mode
act -j test-e2e-k3d --container-options "--network=host"
```

### Issue 5: Workflow Secrets Not Available

**Error:**
```
Secret MY_SECRET not found
```

**Solution**:
```bash
# Pass secrets via command line
act -s MY_SECRET=value

# Or create .secrets file (DO NOT COMMIT!)
echo "MY_SECRET=value" > .secrets
act --secret-file .secrets
```

## Best Practices

### 1. Test Simple Workflows First

Start with workflows that don't require Kubernetes:

```bash
# Test the unit test workflow
act -j test push

# Test the lint workflow
act -j lint push
```

### 2. Use Dry Run for Development

```bash
# See what would run without running it
act -j test-e2e-k3d -n

# Output shows:
# - Steps that would execute
# - Environment variables
# - Commands that would run
```

### 3. Debug Failing Steps

```bash
# Stop on first failure and keep container running
act -j test-e2e-k3d --no-skip-checkout --rm=false

# Then exec into the container
docker exec -it act-... /bin/bash
```

### 4. Cache Dependencies

Act caches Docker images between runs. Speed up subsequent runs:

```bash
# Don't pull images if they exist
act --pull=false

# Reuse containers
act --reuse
```

### 5. Limit Resource Usage

```bash
# Limit memory
act --container-options "--memory=4g"

# Limit CPUs
act --container-options "--cpus=2"
```

## Act vs Real GitHub Actions

### What Works in Act

- ✅ Most workflow syntax
- ✅ Environment variables
- ✅ Secrets (with --secret-file)
- ✅ Matrix builds
- ✅ Service containers
- ✅ Docker actions
- ✅ Composite actions

### What Doesn't Work in Act

- ❌ Self-hosted runners
- ❌ GitHub-specific contexts (e.g., `github.ref`)
- ❌ Artifacts (limited support)
- ❌ Complex Docker-in-Docker scenarios
- ❌ Some GitHub Actions (depends on implementation)

### Limitations with E2E Tests

Our e2e tests create Kubernetes clusters using Docker, which is challenging inside act's Docker container:

**Workaround**:
```bash
# Option 1: Test workflow syntax only
act -j test-e2e-k3d -n

# Option 2: Run e2e tests directly (not through act)
make test-e2e-k3d

# Option 3: Use act for other workflows
act -j lint
act -j test
```

## Workflows That Work Well with Act

### 1. Lint Workflow ✅

```bash
act -j lint push

# Fast, no external dependencies
# Duration: ~30 seconds
```

### 2. Unit Test Workflow ✅

```bash
act -j test push

# Runs unit tests
# Duration: ~1-2 minutes
```

### 3. Build Workflow ✅

```bash
# If you add a build workflow
act -j build push

# Builds Docker images
# Duration: ~2-3 minutes
```

## Advanced Usage

### Running Specific Steps

```bash
# Run only up to a specific step
act -j test-e2e-k3d --step "Create K3D cluster"

# Skip specific steps
act -j test-e2e-k3d --skip "Cleanup K3D cluster"
```

### Using Different Events

```bash
# Simulate pull_request event
act pull_request

# Simulate push to main
act push -e events/push-main.json
```

Create `events/push-main.json`:
```json
{
  "ref": "refs/heads/main",
  "repository": {
    "name": "fabric-x-operator",
    "owner": {
      "login": "kfsoftware"
    }
  }
}
```

### Interactive Debugging

```bash
# Run interactively
act -j test-e2e-k3d -b

# This will:
# - Stop before each step
# - Wait for you to press Enter
# - Allow inspection between steps
```

### Container Shell Access

```bash
# Keep container running after job
act -j test-e2e-k3d --rm=false

# In another terminal, find the container
docker ps -a | grep act

# Exec into it
docker exec -it <container-id> /bin/bash

# Explore the environment
ls -la
env
cat /github/workflow/event.json
```

## Example Workflows

### Test Workflow Syntax

```bash
# Check if workflow is valid
act -l

# Dry run to see steps
act -j test-e2e-k3d -n | less
```

### Test a Workflow Change

```bash
# 1. Edit workflow
vim .github/workflows/test-e2e.yml

# 2. Test locally
act -j test-e2e-k3d -n

# 3. If it looks good, run it
act -j test-e2e-k3d

# 4. Commit and push
git add .github/workflows/test-e2e.yml
git commit -m "feat: update workflow"
git push
```

### Debug a Failing Workflow

```bash
# 1. Run with verbose output
act -j test-e2e-k3d -v 2>&1 | tee act-debug.log

# 2. Keep container running
act -j test-e2e-k3d --rm=false

# 3. Exec into container
docker exec -it $(docker ps -a | grep act | awk '{print $1}' | head -1) /bin/bash

# 4. Manually run failing command
cd /github/workspace
make test-e2e-k3d
```

## CI/CD Testing Workflow

### Development Cycle

```bash
# 1. Make changes to workflow
vim .github/workflows/test-e2e.yml

# 2. Validate syntax
act -l

# 3. Dry run
act -j test-e2e-k3d -n

# 4. Test simple workflows first
act -j lint

# 5. For e2e, run locally
make test-e2e-k3d

# 6. If all good, push
git push
```

## Performance Tips

### Use Image Cache

```bash
# First run (slow)
act -j test-e2e-k3d  # Downloads ~2GB image

# Subsequent runs (fast)
act -j test-e2e-k3d --pull=false  # Uses cached image
```

### Parallel Execution

```bash
# Act doesn't support parallel jobs yet
# Run multiple jobs sequentially:
act -j lint && act -j test
```

### Resource Optimization

```bash
# Use minimal image for simple jobs
act -P ubuntu-latest=node:16-slim -j lint

# Use full image only when needed
act -P ubuntu-latest=catthehacker/ubuntu:full-latest -j test-e2e-k3d
```

## Troubleshooting

### Check Act Logs

```bash
# Verbose output
act -j test-e2e-k3d -v

# Very verbose (debug mode)
act -j test-e2e-k3d -v -v
```

### Check Docker

```bash
# Ensure Docker is running
docker ps

# Check Docker resources
docker info | grep -A 5 "CPUs\|Memory"

# Clean up old act containers
docker ps -a | grep act | awk '{print $1}' | xargs docker rm -f
```

### Reset Act

```bash
# Remove all act containers
docker ps -a | grep act | awk '{print $1}' | xargs docker rm -f

# Remove act images
docker images | grep catthehacker | awk '{print $3}' | xargs docker rmi

# Re-run
act -j test-e2e-k3d
```

## Conclusion

Act is excellent for:
- ✅ Testing workflow syntax
- ✅ Validating simple workflows
- ✅ Quick iteration on CI changes
- ✅ Debugging workflow issues

For complex e2e tests with Kubernetes:
- ⚠️ Use act for validation/dry-run
- ✅ Run actual tests locally with `make test-e2e-k3d`
- ✅ Use GitHub Actions for full integration testing

## Resources

- [Act Documentation](https://github.com/nektos/act)
- [Act Runner Images](https://github.com/catthehacker/docker_images)
- [GitHub Actions Syntax](https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions)
