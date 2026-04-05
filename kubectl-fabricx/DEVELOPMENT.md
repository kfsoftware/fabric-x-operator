# kubectl-fabricx Development Guide

This document provides guidance for developing and extending the kubectl-fabricx plugin.

## Project Structure

```
kubectl-fabricx/
├── main.go                    # Entry point
├── go.mod                     # Go module definition
├── Makefile                   # Build and development tasks
├── README.md                  # User documentation
├── DEVELOPMENT.md             # This file
└── cmd/
    ├── kubectl-fabricx.go    # Main command definition
    ├── helpers/
    │   └── helpers.go        # Common utilities
    ├── ca/                   # CA commands
    │   ├── ca.go            # Main CA command
    │   ├── create.go        # Create CA command
    │   ├── delete.go        # Delete CA command
    │   ├── enroll.go        # Enroll with CA command
    │   ├── register.go      # Register with CA command
    │   └── revoke.go        # Revoke certificate command
    └── peer/                # Peer commands (example)
        ├── peer.go          # Main peer command
        ├── create.go        # Create peer command
        └── delete.go        # Delete peer command
```

## Adding New Commands

### Step 1: Create Command Directory

Create a new directory under `cmd/` for your command:

```bash
mkdir -p cmd/mycommand
```

### Step 2: Create Main Command File

Create a main command file (e.g., `mycommand.go`):

```go
package mycommand

import (
    "github.com/spf13/cobra"
    "io"
)

func NewMyCommandCmd(out io.Writer, errOut io.Writer) *cobra.Command {
    cmd := &cobra.Command{
        Use: "mycommand",
    }
    cmd.AddCommand(newCreateMyCommandCmd(out, errOut))
    cmd.AddCommand(newDeleteMyCommandCmd(out, errOut))
    return cmd
}
```

### Step 3: Create Individual Command Files

Create individual command files for each operation:

#### create.go
```go
package mycommand

import (
    "io"
    "github.com/spf13/cobra"
)

type CreateOptions struct {
    Name      string
    Namespace string
    // Add other options as needed
}

func (o CreateOptions) Validate() error {
    if o.Name == "" {
        return fmt.Errorf("name is required")
    }
    return nil
}

type createCmd struct {
    out    io.Writer
    errOut io.Writer
    opts   CreateOptions
}

func (c *createCmd) validate() error {
    return c.opts.Validate()
}

func (c *createCmd) run(_ []string) error {
    // Implement your command logic here
    return nil
}

func newCreateMyCommandCmd(out io.Writer, errOut io.Writer) *cobra.Command {
    c := &createCmd{
        out:    out,
        errOut: errOut,
    }

    cmd := &cobra.Command{
        Use:   "create",
        Short: "Create a new resource",
        RunE: func(cmd *cobra.Command, args []string) error {
            if err := c.validate(); err != nil {
                return err
            }
            return c.run(args)
        },
    }

    // Add flags
    cmd.Flags().StringVar(&c.opts.Name, "name", "", "Name of the resource")
    cmd.Flags().StringVar(&c.opts.Namespace, "namespace", "default", "Namespace")
    cmd.MarkFlagRequired("name")

    return cmd
}
```

#### delete.go
```go
package mycommand

import (
    "io"
    "github.com/spf13/cobra"
)

func newDeleteMyCommandCmd(out io.Writer, errOut io.Writer) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "delete",
        Short: "Delete a resource",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Implement delete logic
            return nil
        },
    }
    return cmd
}
```

### Step 4: Add to Main Command

Update `cmd/kubectl-fabricx.go` to include your new command:

```go
import (
    "github.com/kfsoftware/fabric-x-operator/kubectl-fabricx/cmd/ca"
    "github.com/kfsoftware/fabric-x-operator/kubectl-fabricx/cmd/peer"
    "github.com/kfsoftware/fabric-x-operator/kubectl-fabricx/cmd/mycommand"  // Add this
    // ... other imports
)

func NewCmdFabricX() *cobra.Command {
    cmd := &cobra.Command{
        Use:          "fabricx",
        Short:        "manage Fabric-X operator CRDs",
        Long:         fabricXDesc,
        SilenceUsage: true,
    }
    logrus.SetLevel(logrus.DebugLevel)
    cmd.AddCommand(
        ca.NewCACmd(cmd.OutOrStdout(), cmd.ErrOrStderr()),
        peer.NewPeerCmd(cmd.OutOrStdout(), cmd.ErrOrStderr()),
        mycommand.NewMyCommandCmd(cmd.OutOrStdout(), cmd.ErrOrStderr()),  // Add this
    )
    return cmd
}
```

## Best Practices

### 1. Command Structure
- Use consistent naming: `new<Command><Action>Cmd`
- Follow the pattern: Options struct → validate → run
- Use proper error handling and logging

### 2. Flag Management
- Use descriptive flag names
- Provide sensible defaults
- Mark required flags with `cmd.MarkFlagRequired()`
- Use appropriate flag types (StringVar, StringSliceVar, BoolVar, etc.)

### 3. Error Handling
- Return meaningful errors
- Use `fmt.Errorf` with context
- Log errors appropriately

### 4. Kubernetes Integration
- Use the helpers package for common operations
- Handle namespace creation if needed
- Use proper resource types from the API

### 5. Testing
- Write unit tests for your commands
- Test error conditions
- Mock Kubernetes client for testing

## Example: Complete Command Implementation

Here's a complete example of a command that creates a resource:

```go
package example

import (
    "context"
    "fmt"
    "io"
    "os/exec"
    "strings"

    "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
    "github.com/kfsoftware/fabric-x-operator/kubectl-fabricx/cmd/helpers"
    log "github.com/sirupsen/logrus"
    "github.com/spf13/cobra"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
)

type CreateOptions struct {
    Name      string
    Namespace string
    Image     string
    Version   string
    Output    bool
}

func (o CreateOptions) Validate() error {
    if o.Name == "" {
        return fmt.Errorf("name is required")
    }
    if o.Namespace == "" {
        o.Namespace = "default"
    }
    return nil
}

type createCmd struct {
    out    io.Writer
    errOut io.Writer
    opts   CreateOptions
}

func (c *createCmd) validate() error {
    return c.opts.Validate()
}

func (c *createCmd) run(_ []string) error {
    // Create your resource
    resource := &v1alpha1.YourResource{
        ObjectMeta: metav1.ObjectMeta{
            Name:      c.opts.Name,
            Namespace: c.opts.Namespace,
        },
        Spec: v1alpha1.YourResourceSpec{
            Image:   c.opts.Image,
            Version: c.opts.Version,
        },
    }

    // Convert to YAML
    yaml, err := helpers.ToYaml([]runtime.Object{resource})
    if err != nil {
        return fmt.Errorf("failed to marshal resource to YAML: %w", err)
    }

    // Apply using kubectl
    args := []string{"apply", "-f", "-"}
    cmd := exec.CommandContext(context.Background(), "kubectl", args...)
    cmd.Stdin = strings.NewReader(yaml[0])
    output, err := cmd.Output()
    if err != nil {
        return fmt.Errorf("failed to create resource: %w", err)
    }

    if c.opts.Output {
        fmt.Fprintf(c.out, "%s\n", string(output))
    } else {
        log.Infof("Resource %s created successfully in namespace %s", c.opts.Name, c.opts.Namespace)
    }

    return nil
}

func newCreateExampleCmd(out io.Writer, errOut io.Writer) *cobra.Command {
    c := &createCmd{
        out:    out,
        errOut: errOut,
    }

    cmd := &cobra.Command{
        Use:   "create",
        Short: "Create a new example resource",
        RunE: func(cmd *cobra.Command, args []string) error {
            if err := c.validate(); err != nil {
                return err
            }
            return c.run(args)
        },
    }

    cmd.Flags().StringVar(&c.opts.Name, "name", "", "Name of the resource")
    cmd.Flags().StringVar(&c.opts.Namespace, "namespace", "default", "Namespace")
    cmd.Flags().StringVar(&c.opts.Image, "image", "example:latest", "Image")
    cmd.Flags().StringVar(&c.opts.Version, "version", "1.0.0", "Version")
    cmd.Flags().BoolVar(&c.opts.Output, "output", false, "Output YAML instead of applying")

    cmd.MarkFlagRequired("name")

    return cmd
}
```

## Building and Testing

### Build
```bash
make build
# or
go build -o kubectl-fabricx .
```

### Test
```bash
make test
# or
go test ./...
```

### Install
```bash
make install
# or
cp kubectl-fabricx ~/.local/bin/
```

## Contributing

1. Follow the existing code structure and patterns
2. Add appropriate tests for new functionality
3. Update documentation
4. Ensure the plugin builds successfully
5. Test the plugin with actual Kubernetes clusters 