package peer

import (
	"io"

	"github.com/spf13/cobra"
)

func NewPeerCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use: "peer",
	}
	cmd.AddCommand(newCreatePeerCmd(out, errOut))
	cmd.AddCommand(newPeerDeleteCmd(out, errOut))
	return cmd
}
