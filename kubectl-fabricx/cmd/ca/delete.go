package ca

import (
	"io"

	"github.com/spf13/cobra"
)

func newCADeleteCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a Fabric CA",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement delete functionality
			return nil
		},
	}
	return cmd
}
