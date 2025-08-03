package ca

import (
	"io"

	"github.com/spf13/cobra"
)

func newCARevokeCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke a certificate with a Fabric CA",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement revoke functionality
			return nil
		},
	}
	return cmd
}
