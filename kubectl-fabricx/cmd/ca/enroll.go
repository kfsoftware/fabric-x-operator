package ca

import (
	"io"

	"github.com/spf13/cobra"
)

func newCAEnrollCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll with a Fabric CA",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Implement enroll functionality
			return nil
		},
	}
	return cmd
}
