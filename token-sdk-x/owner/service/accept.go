//go:build fabricx

package service

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/ttx"
)

// AcceptCashView is the recipient's view for incoming token transactions
// (issue or transfer). It responds to the identity request, receives the
// transaction, accepts it, and waits for finality.
type AcceptCashView struct{}

func (a *AcceptCashView) Call(context view.Context) (interface{}, error) {
	// Step 1: Respond to the sender's request for a recipient identity.
	_, err := ttx.RespondRequestRecipientIdentityUsingWallet(context, "")
	if err != nil {
		return nil, fmt.Errorf("failed to respond to identity request: %w", err)
	}

	// Step 2: Receive the assembled transaction from the sender.
	tx, err := ttx.ReceiveTransaction(context)
	if err != nil {
		return nil, fmt.Errorf("failed to receive transaction: %w", err)
	}

	// Step 3: Accept and return signature.
	_, err = context.RunView(ttx.NewAcceptView(tx))
	if err != nil {
		return nil, fmt.Errorf("failed to accept transaction: %w", err)
	}

	// Step 4: Wait for finality.
	_, err = context.RunView(ttx.NewFinalityView(tx))
	if err != nil {
		return nil, fmt.Errorf("finality failed: %w", err)
	}

	return nil, nil
}
