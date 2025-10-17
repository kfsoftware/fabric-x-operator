# Running Tests in VSCode

This guide shows how to run and debug Ginkgo tests for the fabric-x-operator using VSCode.

## Prerequisites

- VSCode with the official Go extension (`golang.go`) installed
- Kubebuilder test environment set up (run `make envtest` if not already done)

## Test Execution Methods

### 1. Using CodeLens (Easiest Method)

The Go extension automatically adds "run test" and "debug test" links above each test in your test files.

1. Open [internal/controller/endorser_controller_test.go](../internal/controller/endorser_controller_test.go)
2. Look for the `run test | debug test` links above each `Describe`, `Context`, or `It` block
3. Click **"run test"** to execute the test
4. Click **"debug test"** to run with breakpoints

Example:
```go
// run test | debug test   <- Click these links
var _ = Describe("Endorser Controller", func() {
    // test code...
})
```

### 2. Using Test Explorer

The Go Test Explorer shows all tests in a tree view:

1. Open the **Testing** view (test tube icon in the left sidebar)
2. Expand the test tree to find your tests
3. Click the **play button** next to any test to run it
4. Click the **bug icon** to debug
5. Use the **refresh button** if tests don't appear

### 3. Using Command Palette

Quick test execution without leaving your keyboard:

1. Press `Cmd+Shift+P` (macOS) or `Ctrl+Shift+P` (Linux/Windows)
2. Type "Go: Test"
3. Choose from:
   - **Go: Test Function At Cursor** - Run the test where your cursor is
   - **Go: Test File** - Run all tests in current file
   - **Go: Test Package** - Run all tests in the package
   - **Go: Test All Packages in Workspace** - Run all tests

### 4. Using Debug Configurations

For advanced debugging scenarios, use the pre-configured launch configurations:

1. Open the **Run and Debug** view (`Cmd+Shift+D`)
2. Select a configuration from the dropdown:
   - **Debug Endorser Controller Tests** - Run all endorser tests
   - **Debug Current Test File** - Debug the currently open test file
   - **Debug Specific Test (with focus)** - Run a specific test by name
   - **Run All Controller Tests** - Run all controller tests

3. Press `F5` or click the green play button

#### Debug Specific Test Example

To debug just one test:

1. Select **"Debug Specific Test (with focus)"**
2. Press `F5`
3. Enter the test description when prompted, e.g.:
   - `should successfully reconcile a basic endorser in configure mode`
   - `should create deployment in deploy mode`
   - `should handle invalid bootstrap mode`

This uses Ginkgo's `-ginkgo.focus` flag to run only matching tests.

## Running Tests from Terminal

You can also run tests from VSCode's integrated terminal:

### Run all endorser tests:
```bash
go test ./internal/controller -v -ginkgo.focus="Endorser Controller"
```

### Run a specific test:
```bash
go test ./internal/controller -v -ginkgo.focus="should successfully reconcile"
```

### Run with coverage:
```bash
go test ./internal/controller -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run all tests in the project:
```bash
make test
```

## Debugging Tips

### Setting Breakpoints

1. Click in the gutter (left of line numbers) to set a breakpoint (red dot)
2. Start debugging using any of the methods above
3. Execution will pause at breakpoints
4. Use the debug toolbar to:
   - **Continue** (F5)
   - **Step Over** (F10)
   - **Step Into** (F11)
   - **Step Out** (Shift+F11)

### Watching Variables

1. When paused at a breakpoint, hover over variables to see values
2. Use the **Variables** panel in the debug sidebar
3. Add expressions to the **Watch** panel
4. Inspect complex objects in the **Debug Console**

### Common Ginkgo Commands

Add these flags to debug configurations or terminal commands:

- `-ginkgo.v` - Verbose output
- `-ginkgo.focus="test description"` - Run specific tests
- `-ginkgo.skip="test description"` - Skip specific tests
- `-ginkgo.progress` - Show progress during test execution
- `-ginkgo.trace` - Show full stack traces
- `-ginkgo.failFast` - Stop on first failure

## Troubleshooting

### Tests not appearing in Test Explorer

1. Make sure you're in a Go workspace
2. Reload the window: `Cmd+Shift+P` → "Developer: Reload Window"
3. Check that `go.testExplorer.enable` is true in settings

### KUBEBUILDER_ASSETS error

If you see "unable to start control plane", run:
```bash
make envtest
```

This downloads the required Kubernetes binaries.

### Tests timeout

Increase timeout in [.vscode/settings.json](../.vscode/settings.json):
```json
{
  "go.testTimeout": "30m"
}
```

### Can't debug into dependencies

Add this to your launch configuration:
```json
{
  "dlvFlags": ["--check-go-version=false"]
}
```

## VSCode Extensions (Optional)

While the official Go extension handles Ginkgo tests well, you can optionally install:

- **Coverage Gutters** - Show test coverage in editor gutters
- **Error Lens** - Inline error messages
- **Test Explorer UI** - Enhanced test explorer (if you prefer a different UI)

Install via:
```bash
code --install-extension ryanluker.vscode-coverage-gutters
code --install-extension usernamehw.errorlens
```

## Configuration Files

- [.vscode/settings.json](../.vscode/settings.json) - Go test configuration
- [.vscode/launch.json](../.vscode/launch.json) - Debug configurations

## Example Workflow

1. Open [internal/controller/endorser_controller_test.go](../internal/controller/endorser_controller_test.go)
2. Set a breakpoint in the test you want to debug
3. Click "debug test" above the `It` block, or
4. Press `Cmd+Shift+D`, select "Debug Current Test File", and press `F5`
5. Examine variables and step through code
6. Make changes to the controller
7. Click "run test" to verify the fix
8. Repeat until all tests pass

## Related Documentation

- [ENDORSER_IMPLEMENTATION.md](../ENDORSER_IMPLEMENTATION.md) - Endorser controller details
- [Ginkgo Documentation](https://onsi.github.io/ginkgo/)
- [VSCode Go Extension](https://github.com/golang/vscode-go)
