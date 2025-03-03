package ante

import (
	"fmt"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"runtime/debug"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"

	errorsmod "cosmossdk.io/errors"
	tmlog "cosmossdk.io/log"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
)

func ValidateHandlerOptions(options HandlerOptions) error {
	if options.AccountKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "account keeper is required for ante builder")
	}
	if options.BankKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "bank keeper is required for ante builder")
	}
	if options.SignModeHandler == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "sign mode handler is required for ante builder")
	}
	if options.NodeConfig == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "wasm config is required for ante builder")
	}
	if options.TXCounterStoreService == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "wasm store service is required for ante builder")
	}
	if options.CircuitKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "circuit keeper is required for ante builder")
	}
	if options.FeeMarketKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "fee market keeper is required for AnteHandler")
	}
	if options.EvmKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "evm keeper is required for AnteHandler")
	}
	if options.WasmKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "wasm keeper is required for AnteHandler")
	}
	if options.IBCKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "ibc keeper is required for AnteHandler")
	}

	return nil
}

// NewAnteHandler returns an ante handler responsible for attempting to route an
// Ethereum or SDK transaction to an internal ante handler for performing
// transaction-level processing (e.g. fee payment, signature verification) before
// being passed onto it's respective handler.
func NewAnteHandler(options HandlerOptions) (sdk.AnteHandler, error) {
	if err := ValidateHandlerOptions(options); err != nil {
		return nil, err
	}

	return func(
		ctx sdk.Context, tx sdk.Tx, sim bool,
	) (newCtx sdk.Context, err error) {
		var anteHandler sdk.AnteHandler

		defer Recover(ctx.Logger(), &err)

		txWithExtensions, ok := tx.(authante.HasExtensionOptionsTx)
		if ok {
			opts := txWithExtensions.GetExtensionOptions()
			if len(opts) > 0 {
				switch typeURL := opts[0].GetTypeUrl(); typeURL {
				case "/ethermint.evm.v1.ExtensionOptionsEthereumTx":
					// handle as *evmtypes.MsgEthereumTx
					anteHandler = newEthAnteHandler(options)
				case "/ethermint.types.v1.ExtensionOptionsWeb3Tx":
					// Deprecated: Handle as normal Cosmos SDK tx, except signature is checked for Legacy EIP712 representation
					anteHandler = NewLegacyCosmosAnteHandlerEip712(options)
				case "/ethermint.types.v1.ExtensionOptionDynamicFeeTx":
					// cosmos-sdk tx with dynamic fee extension
					anteHandler = newCosmosAnteHandler(options)
				default:
					return ctx, errorsmod.Wrapf(
						errortypes.ErrUnknownExtensionOptions,
						"rejecting tx with unsupported extension option: %s", typeURL,
					)
				}

				return anteHandler(ctx, tx, sim)
			}
		}

		// handle as totally normal Cosmos SDK tx
		switch tx.(type) {
		case sdk.Tx:
			// default: handle as normal Cosmos SDK tx
			anteHandler = newCosmosAnteHandler(options)

			// if tx is a system tx, and singer is authorized, use system tx handler

			// TODO revisit SystemTx implementation and usage
			isAuthorized := func(creator string) bool {
				return false
			}
			if IsSystemTx(tx, isAuthorized) {
				anteHandler = newCosmosAnteHandlerForSystemTx(options)
			}

			// if tx is MsgCreatorValidator, use the newCosmosAnteHandlerForSystemTx handler to
			// exempt gas fee requirement in genesis because it's not possible to pay gas fee in genesis
			if len(tx.GetMsgs()) == 1 {
				if _, ok := tx.GetMsgs()[0].(*stakingtypes.MsgCreateValidator); ok && ctx.BlockHeight() == 0 {
					anteHandler = newCosmosAnteHandlerForSystemTx(options)
				}
			}

		default:
			return ctx, errorsmod.Wrapf(errortypes.ErrUnknownRequest, "invalid transaction type: %T", tx)
		}

		return anteHandler(ctx, tx, sim)
	}, nil
}

func Recover(logger tmlog.Logger, err *error) {
	if r := recover(); r != nil {
		if err != nil {
			// #nosec G703 err is checked non-nil above
			*err = errorsmod.Wrapf(errortypes.ErrPanic, "%v", r)
		}

		if e, ok := r.(error); ok {
			logger.Error(
				"ante handler panicked",
				"error", e,
				"stack trace", string(debug.Stack()),
			)
		} else {
			logger.Error(
				"ante handler panicked",
				"recover", fmt.Sprintf("%v", r),
			)
		}
	}
}

// IsSystemTx determines whether tx is a system tx that's signed by an authorized signer
// system tx are special types of txs (see in the switch below), or such txs wrapped inside a MsgExec
// the parameter isAuthorizedSigner is a caller specified function that determines whether the signer of
// the tx is authorized.
func IsSystemTx(tx sdk.Tx, isAuthorizedSigner func(string) bool) bool {
	// TODO revisit SystemTx implementation and usage

	// the following determines whether the tx is a system tx which will uses different handler
	// System txs are always single Msg txs, optionally wrapped by one level of MsgExec
	if len(tx.GetMsgs()) != 1 { // this is not a system tx
		return false
	}
	msg := tx.GetMsgs()[0]

	// if wrapped inside a MsgExec, unwrap it and reveal the innerMsg.
	var innerMsg sdk.Msg
	innerMsg = msg
	if mm, ok := msg.(*authz.MsgExec); ok { // authz tx; look inside it
		msgs, err := mm.GetMessages()
		if err == nil && len(msgs) == 1 {
			innerMsg = msgs[0]
		}
	}
	switch innerMsg.(type) {
	case *wasmtypes.MsgExecuteContract:
		signers := innerMsg.(sdk.LegacyMsg).GetSigners()
		if len(signers) == 1 {
			return isAuthorizedSigner(signers[0].String())
		}
	}

	return false
}
