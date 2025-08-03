package cmd

import (
	"github.com/kfsoftware/fabric-x-operator/kubectl-fabricx/cmd/ca"
	"github.com/kfsoftware/fabric-x-operator/kubectl-fabricx/cmd/peer"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	// Workaround for authentication plugins https://krew.sigs.k8s.io/docs/developer-guide/develop/best-practices/#auth-plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	fabricXDesc = `
kubectl plugin to manage Fabric-X operator CRDs.`
)

// NewCmdFabricX creates a new root command for kubectl-fabricx
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
		// TODO: Add more commands here as they are implemented
		// orderer.NewOrdererCmd(cmd.OutOrStdout(), cmd.ErrOrStderr()),
	)
	return cmd
}
