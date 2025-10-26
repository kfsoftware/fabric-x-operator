# GitHub Actions Workflows

This directory contains CI/CD workflows for the Fabric-X Operator project.

## Workflows

### test-e2e.yml - E2E Testing

Runs end-to-end tests on both Kind and K3D clusters to ensure compatibility across different Kubernetes distributions.

**Jobs:**

1. **test-e2e-kind**: Tests with Kind
   - Platform: ubuntu-latest
   - Cluster: Kind (Kubernetes in Docker)
   - Creates temporary cluster, runs tests, cleans up
   - Duration: ~3-4 minutes

2. **test-e2e-k3d**: Tests with K3D
   - Platform: ubuntu-latest
   - Cluster: K3D with 2 agents
   - Creates temporary cluster, runs tests, cleans up
   - Duration: ~2-3 minutes

**Triggers:**
- Push to any branch
- Pull requests

**What it tests:**
- Building the operator Docker image
- Loading image to cluster
- Installing CRDs
- Deploying the operator
- Controller manager functionality
- Metrics endpoint serving

### test.yml - Unit Tests

Runs the unit test suite.

**What it tests:**
- Controller unit tests
- API validation
- Helper functions
- Business logic

### lint.yml - Code Quality

Runs linting and static analysis.

**Tools:**
- golangci-lint
- Code formatting checks
- Import order checks

## Running Workflows Locally

### E2E Tests (Kind)
```bash
make test-e2e
```

### E2E Tests (K3D)
```bash
# With existing K3D cluster
export K3D_CLUSTER=my-cluster
make test-e2e-k3d

# Or let it use default (k8s-hlf)
make test-e2e-k3d
```

### Unit Tests
```bash
make test
```

### Linting
```bash
make lint
```

## CI/CD Best Practices

### When to Run E2E Tests

E2E tests should run:
- ✅ On every PR (automated)
- ✅ Before merging to main
- ✅ On release branches
- ✅ Nightly builds (if configured)

### When to Skip E2E Tests

Use `[skip ci]` in commit message to skip all workflows:
```bash
git commit -m "docs: update README [skip ci]"
```

Or skip specific workflows using paths filters (not currently configured).

### Debugging Failed Workflows

1. **View workflow run:**
   - Go to Actions tab in GitHub
   - Click on failed workflow
   - Click on failed job

2. **Check artifacts:**
   - Look for uploaded test logs/artifacts
   - Currently no artifacts uploaded (can be added)

3. **Reproduce locally:**
   ```bash
   # Run the exact same test that failed
   make test-e2e-k3d
   ```

4. **Common failures:**
   - **Timeout**: Increase timeout in Makefile
   - **Image pull**: Pre-pull images or check registry
   - **Cluster creation**: Check Docker/K3D installation
   - **CertManager**: May need to skip installation

## Workflow Configuration

### Timeouts

All e2e test jobs have appropriate timeouts:
- Go test timeout: 10 minutes
- K3D cluster creation: 5 minutes
- Job timeout: 30 minutes (GitHub default)

### Environment Variables

Set these in workflow `env:` section:
- `CLUSTER_TYPE`: "k3d" or "kind"
- `K3D_CLUSTER`: K3D cluster name
- `KIND_CLUSTER`: Kind cluster name
- `CERT_MANAGER_INSTALL_SKIP`: "true" to skip CertManager

Example:
```yaml
- name: Run tests
  env:
    CLUSTER_TYPE: k3d
    K3D_CLUSTER: test-cluster
  run: make test-e2e-k3d
```

## Adding New Workflows

To add a new workflow:

1. Create `.github/workflows/my-workflow.yml`
2. Define jobs and steps
3. Test locally first
4. Commit and push
5. Check Actions tab for results

Example workflow:
```yaml
name: My Workflow

on:
  push:
  pull_request:

jobs:
  my-job:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: make my-target
```

## Performance Tips

### Faster CI Runs

1. **Use caching:**
   ```yaml
   - uses: actions/cache@v3
     with:
       path: |
         ~/.cache/go-build
         ~/go/pkg/mod
       key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
   ```

2. **Run jobs in parallel:**
   - E2E tests on Kind and K3D run in parallel
   - Add more matrix builds if needed

3. **Skip unnecessary steps:**
   - Use `if:` conditions
   - Skip cleanup if tests fail for debugging

4. **Pre-pull images:**
   ```yaml
   - name: Pre-pull images
     run: |
       docker pull curlimages/curl:latest
       docker pull gcr.io/kubebuilder/kube-rbac-proxy:v0.8.0
   ```

## Monitoring

### Workflow Status Badges

Add to README.md:
```markdown
![E2E Tests](https://github.com/YOUR_ORG/fabric-x-operator/actions/workflows/test-e2e.yml/badge.svg)
![Unit Tests](https://github.com/YOUR_ORG/fabric-x-operator/actions/workflows/test.yml/badge.svg)
![Lint](https://github.com/YOUR_ORG/fabric-x-operator/actions/workflows/lint.yml/badge.svg)
```

### Notifications

Configure GitHub to send notifications:
- Settings → Notifications
- Or use Slack/Discord integrations

## Troubleshooting

### Problem: Workflow fails with "cluster not found"

**Solution:** Check cluster creation step logs. Increase timeout or check Docker service.

### Problem: Tests pass locally but fail in CI

**Solution:**
- Check environment differences
- Verify all dependencies are installed
- Check timeout values
- Look at CI logs for specific errors

### Problem: K3D cluster creation hangs

**Solution:**
- Increase `--timeout` value
- Check Docker daemon status
- Reduce number of agents
- Try Kind instead

### Problem: Image pull errors

**Solution:**
- Use public images
- Add image pull secrets if using private registry
- Pre-pull images before tests

## Security Considerations

### Secrets Management

Never commit secrets to workflows. Use GitHub Secrets:
```yaml
env:
  MY_SECRET: ${{ secrets.MY_SECRET }}
```

### Permissions

Workflows run with restricted permissions by default. Add permissions if needed:
```yaml
permissions:
  contents: read
  packages: write
```

## Maintenance

### Regular Updates

Update these regularly:
- Go version in `actions/setup-go`
- Action versions (`@v4` → `@v5`)
- K3D/Kind versions
- Tool versions (golangci-lint, etc.)

### Deprecation Notices

Watch for GitHub Actions deprecation notices:
- Node.js version updates
- Action version updates
- Workflow syntax changes
