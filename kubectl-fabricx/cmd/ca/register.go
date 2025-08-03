package ca

import (
	"io"

	"github.com/spf13/cobra"
)

func newCARegisterCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register with a Fabric CA",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement register functionality
			return nil
		},
	}
	return cmd
}
