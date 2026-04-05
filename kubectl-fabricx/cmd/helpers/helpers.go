package helpers

import (
	"bytes"
	"context"
	"io"
	"os/exec"

	fabricxclientset "github.com/kfsoftware/fabric-x-operator/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"
)

// GetKubeClient provides k8s client for kubeconfig
func GetKubeClient() (*kubernetes.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	kubeClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return kubeClientset, nil
}

// GetControllerRuntimeClient provides a controller-runtime client
func GetControllerRuntimeClient(scheme *runtime.Scheme) (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kubeconfig")
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create client")
	}
	return k8sClient, nil
}

// GetFabricXClient provides a typed Fabric X clientset for type-safe operations
func GetFabricXClient() (fabricxclientset.Interface, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kubeconfig")
	}

	fabricxClient, err := fabricxclientset.NewForConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Fabric X client")
	}
	return fabricxClient, nil
}

// ExecKubectl executes the given command using `kubectl`
func ExecKubectl(ctx context.Context, args ...string) ([]byte, error) {
	var stdout, stderr, combined bytes.Buffer

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdout = io.MultiWriter(&stdout, &combined)
	cmd.Stderr = io.MultiWriter(&stderr, &combined)
	if err := cmd.Run(); err != nil {
		return nil, errors.Errorf("kubectl command failed (%s). output=%s", err, combined.String())
	}
	return stdout.Bytes(), nil
}

// ToYaml takes a slice of values, and returns corresponding YAML
// representation as a string slice
func ToYaml(objs []runtime.Object) ([]string, error) {
	manifests := make([]string, len(objs))
	for i, obj := range objs {
		o, err := yaml.Marshal(obj)
		if err != nil {
			return []string{}, err
		}
		manifests[i] = string(o)
	}

	return manifests, nil
}
