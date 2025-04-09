package wasmbinding

import (
	"encoding/json"
	"fmt"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	paramChange "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	contractmanagerkeeper "github.com/neutron-org/neutron/v6/x/contractmanager/keeper"
	cronkeeper "github.com/neutron-org/neutron/v6/x/cron/keeper"
	crontypes "github.com/neutron-org/neutron/v6/x/cron/types"

	"github.com/cosmos/cosmos-sdk/codec/types"

	errorsmod "cosmossdk.io/errors"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmvmtypes "github.com/CosmWasm/wasmvm/v2/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"

	"github.com/cyborgshead/cyber-rollup/wasmbinding/bindings"

	tokenfactorykeeper "github.com/neutron-org/neutron/v6/x/tokenfactory/keeper"
	tokenfactorytypes "github.com/neutron-org/neutron/v6/x/tokenfactory/types"

	transferwrapperkeeper "github.com/neutron-org/neutron/v6/x/transfer/keeper"
	transferwrappertypes "github.com/neutron-org/neutron/v6/x/transfer/types"

	adminmodulekeeper "github.com/cosmos/admin-module/v2/x/adminmodule/keeper"
	admintypes "github.com/cosmos/admin-module/v2/x/adminmodule/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	contractmanagertypes "github.com/neutron-org/neutron/v6/x/contractmanager/types"
)

// CustomMessageDecorator returns decorator for custom CosmWasm bindings messages
func CustomMessageDecorator(
	bank *bankkeeper.BaseKeeper,
	tokenFactory *tokenfactorykeeper.Keeper,
	cronKeeper *cronkeeper.Keeper,
	adminKeeper *adminmodulekeeper.Keeper,
	transferKeeper transferwrapperkeeper.KeeperTransferWrapper,
	contractmanagerKeeper *contractmanagerkeeper.Keeper,
) func(wasmkeeper.Messenger) wasmkeeper.Messenger {
	return func(old wasmkeeper.Messenger) wasmkeeper.Messenger {
		return &CustomMessenger{
			Wrapped:                    old,
			Bank:                       bank,
			TokenFactory:               tokenFactory,
			CronMsgServer:              cronkeeper.NewMsgServerImpl(*cronKeeper),
			CronQueryServer:            cronKeeper,
			AdminKeeper:                adminKeeper,
			Adminserver:                adminmodulekeeper.NewMsgServerImpl(*adminKeeper),
			transferKeeper:             transferKeeper,
			ContractmanagerMsgServer:   contractmanagerkeeper.NewMsgServerImpl(*contractmanagerKeeper),
			ContractmanagerQueryServer: contractmanagerkeeper.NewQueryServerImpl(*contractmanagerKeeper),
		}
	}
}

type CustomMessenger struct {
	Wrapped                    wasmkeeper.Messenger
	Bank                       *bankkeeper.BaseKeeper
	TokenFactory               *tokenfactorykeeper.Keeper
	CronMsgServer              crontypes.MsgServer
	CronQueryServer            crontypes.QueryServer
	AdminKeeper                *adminmodulekeeper.Keeper
	Adminserver                admintypes.MsgServer
	transferKeeper             transferwrapperkeeper.KeeperTransferWrapper
	ContractmanagerMsgServer   contractmanagertypes.MsgServer
	ContractmanagerQueryServer contractmanagertypes.QueryServer
}

var _ wasmkeeper.Messenger = (*CustomMessenger)(nil)

// DispatchMsg executes on the contractMsg.
func (m *CustomMessenger) DispatchMsg(ctx sdk.Context, contractAddr sdk.AccAddress, contractIBCPortID string, msg wasmvmtypes.CosmosMsg) (events []sdk.Event, data [][]byte, msgResponses [][]*codectypes.Any, err error) {
	// Return early if msg.Custom is nil
	if msg.Custom == nil {
		return m.Wrapped.DispatchMsg(ctx, contractAddr, contractIBCPortID, msg)
	}

	var contractMsg bindings.CyberMsg
	if err := json.Unmarshal(msg.Custom, &contractMsg); err != nil {
		ctx.Logger().Debug("json.Unmarshal: failed to decode incoming custom cosmos message",
			"from_address", contractAddr.String(),
			"message", string(msg.Custom),
			"error", err,
		)
		return nil, nil, nil, errorsmod.Wrap(err, "failed to decode incoming custom cosmos message")
	}
	if contractMsg.IBCTransfer != nil {
		return m.ibcTransfer(ctx, contractAddr, *contractMsg.IBCTransfer)
	}
	if contractMsg.SubmitAdminProposal != nil {
		return m.submitAdminProposal(ctx, contractAddr, &contractMsg.SubmitAdminProposal.AdminProposal)
	}
	if contractMsg.CreateDenom != nil {
		return m.createDenom(ctx, contractAddr, contractMsg.CreateDenom)
	}
	if contractMsg.MintTokens != nil {
		return m.mintTokens(ctx, contractAddr, contractMsg.MintTokens)
	}
	if contractMsg.ChangeAdmin != nil {
		return m.changeAdmin(ctx, contractAddr, contractMsg.ChangeAdmin)
	}
	if contractMsg.BurnTokens != nil {
		return m.burnTokens(ctx, contractAddr, contractMsg.BurnTokens)
	}
	if contractMsg.SetBeforeSendHook != nil {
		return m.setBeforeSendHook(ctx, contractAddr, contractMsg.SetBeforeSendHook)
	}
	if contractMsg.ForceTransfer != nil {
		return m.forceTransfer(ctx, contractAddr, contractMsg.ForceTransfer)
	}
	if contractMsg.SetDenomMetadata != nil {
		return m.setDenomMetadata(ctx, contractAddr, contractMsg.SetDenomMetadata)
	}

	if contractMsg.AddSchedule != nil {
		return m.addSchedule(ctx, contractAddr, contractMsg.AddSchedule)
	}
	if contractMsg.RemoveSchedule != nil {
		return m.removeSchedule(ctx, contractAddr, contractMsg.RemoveSchedule)
	}
	if contractMsg.ResubmitFailure != nil {
		return m.resubmitFailure(ctx, contractAddr, contractMsg.ResubmitFailure)
	}

	return m.Wrapped.DispatchMsg(ctx, contractAddr, contractIBCPortID, msg)
}

func (m *CustomMessenger) ibcTransfer(ctx sdk.Context, contractAddr sdk.AccAddress, ibcTransferMsg transferwrappertypes.MsgTransfer) ([]sdk.Event, [][]byte, [][]*types.Any, error) {
	ibcTransferMsg.Sender = contractAddr.String()

	response, err := m.transferKeeper.Transfer(ctx, &ibcTransferMsg)
	if err != nil {
		ctx.Logger().Debug("transferServer.Transfer: failed to transfer",
			"from_address", contractAddr.String(),
			"msg", ibcTransferMsg,
			"error", err,
		)
		return nil, nil, nil, errorsmod.Wrap(err, "failed to execute IBCTransfer")
	}

	data, err := json.Marshal(response)
	if err != nil {
		ctx.Logger().Error("json.Marshal: failed to marshal MsgTransferResponse response to JSON",
			"from_address", contractAddr.String(),
			"msg", response,
			"error", err,
		)
		return nil, nil, nil, errorsmod.Wrap(err, "marshal json failed")
	}

	ctx.Logger().Debug("ibcTransferMsg completed",
		"from_address", contractAddr.String(),
		"msg", ibcTransferMsg,
	)

	anyResp, err := types.NewAnyWithValue(response)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrapf(err, "failed to convert {%T} to Any", response)
	}
	msgResponses := [][]*types.Any{{anyResp}}
	return nil, [][]byte{data}, msgResponses, nil
}

func (m *CustomMessenger) submitAdminProposal(ctx sdk.Context, contractAddr sdk.AccAddress, adminProposal *bindings.AdminProposal) ([]sdk.Event, [][]byte, [][]*types.Any, error) {
	var data []byte
	err := m.validateProposalQty(adminProposal)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrap(err, "invalid proposal quantity")
	}
	// here we handle pre-v2.0.0 style of proposals: param change, upgrade, client update
	if m.isLegacyProposal(adminProposal) {
		resp, err := m.performSubmitAdminProposalLegacy(ctx, contractAddr, adminProposal)
		if err != nil {
			ctx.Logger().Debug("performSubmitAdminProposalLegacy: failed to submitAdminProposal",
				"from_address", contractAddr.String(),
				"error", err,
			)
			return nil, nil, nil, errorsmod.Wrap(err, "failed to submit admin proposal legacy")
		}
		data, err = json.Marshal(resp)
		if err != nil {
			ctx.Logger().Error("json.Marshal: failed to marshal submitAdminProposalLegacy response to JSON",
				"from_address", contractAddr.String(),
				"error", err,
			)
			return nil, nil, nil, errorsmod.Wrap(err, "marshal json failed")
		}

		ctx.Logger().Debug("submit proposal legacy submitted",
			"from_address", contractAddr.String(),
		)

		anyResp, err := types.NewAnyWithValue(resp)
		if err != nil {
			return nil, nil, nil, errorsmod.Wrapf(err, "failed to convert {%T} to Any", resp)
		}
		msgResponses := [][]*types.Any{{anyResp}}
		return nil, [][]byte{data}, msgResponses, nil
	}

	resp, err := m.performSubmitAdminProposal(ctx, contractAddr, adminProposal)
	if err != nil {
		ctx.Logger().Debug("performSubmitAdminProposal: failed to submitAdminProposal",
			"from_address", contractAddr.String(),
			"error", err,
		)
		return nil, nil, nil, errorsmod.Wrap(err, "failed to submit admin proposal")
	}

	data, err = json.Marshal(resp)
	if err != nil {
		ctx.Logger().Error("json.Marshal: failed to marshal submitAdminProposal response to JSON",
			"from_address", contractAddr.String(),
			"error", err,
		)
		return nil, nil, nil, errorsmod.Wrap(err, "marshal json failed")
	}

	ctx.Logger().Debug("submit proposal message submitted",
		"from_address", contractAddr.String(),
	)

	anyResp, err := types.NewAnyWithValue(resp)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrapf(err, "failed to convert {%T} to Any", resp)
	}
	msgResponses := [][]*types.Any{{anyResp}}
	return nil, [][]byte{data}, msgResponses, nil
}

func (m *CustomMessenger) performSubmitAdminProposalLegacy(ctx sdk.Context, contractAddr sdk.AccAddress, adminProposal *bindings.AdminProposal) (*admintypes.MsgSubmitProposalLegacyResponse, error) {
	proposal := adminProposal
	msg := admintypes.MsgSubmitProposalLegacy{Proposer: contractAddr.String()}

	switch {
	case proposal.ParamChangeProposal != nil:
		p := proposal.ParamChangeProposal
		err := msg.SetContent(&paramChange.ParameterChangeProposal{
			Title:       p.Title,
			Description: p.Description,
			Changes:     p.ParamChanges,
		})
		if err != nil {
			return nil, errorsmod.Wrap(err, "failed to set content on ParameterChangeProposal")
		}
	default:
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unexpected legacy admin proposal structure: %+v", proposal)
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, errorsmod.Wrap(err, "failed to validate incoming SubmitAdminProposal message")
	}

	response, err := m.Adminserver.SubmitProposalLegacy(ctx, &msg)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to submit proposal")
	}

	ctx.Logger().Debug("submit proposal legacy processed in msg server",
		"from_address", contractAddr.String(),
	)

	return response, nil
}

func (m *CustomMessenger) performSubmitAdminProposal(ctx sdk.Context, contractAddr sdk.AccAddress, adminProposal *bindings.AdminProposal) (*admintypes.MsgSubmitProposalResponse, error) {
	proposal := adminProposal
	authority := authtypes.NewModuleAddress(admintypes.ModuleName)
	var (
		msg    *admintypes.MsgSubmitProposal
		sdkMsg sdk.Msg
	)

	cdc := m.AdminKeeper.Codec()
	err := cdc.UnmarshalInterfaceJSON([]byte(proposal.ProposalExecuteMessage.Message), &sdkMsg)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to unmarshall incoming sdk message")
	}

	signers, _, err := cdc.GetMsgV1Signers(sdkMsg)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to get signers from incoming sdk message")
	}
	if len(signers) != 1 {
		return nil, errorsmod.Wrap(sdkerrors.ErrorInvalidSigner, "should be 1 signer")
	}
	if !sdk.AccAddress(signers[0]).Equals(authority) {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "authority in incoming msg is not equal to admin module")
	}

	msg, err = admintypes.NewMsgSubmitProposal([]sdk.Msg{sdkMsg}, contractAddr)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to create MsgSubmitProposal ")
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, errorsmod.Wrap(err, "failed to validate incoming SubmitAdminProposal message")
	}

	response, err := m.Adminserver.SubmitProposal(ctx, msg)
	if err != nil {
		return nil, errorsmod.Wrap(err, "failed to submit proposal")
	}

	return response, nil
}

func (m *CustomMessenger) createDenom(ctx sdk.Context, contractAddr sdk.AccAddress, createDenom *bindings.CreateDenom) (events []sdk.Event, data [][]byte, msgResponses [][]*codectypes.Any, err error) {
	err = PerformCreateDenom(m.TokenFactory, m.Bank, ctx, contractAddr, createDenom)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrap(err, "perform create denom")
	}
	return nil, nil, nil, nil
}

// PerformCreateDenom is used with createDenom to create a token denom; validates the msgCreateDenom.
func PerformCreateDenom(f *tokenfactorykeeper.Keeper, _ *bankkeeper.BaseKeeper, ctx sdk.Context, contractAddr sdk.AccAddress, createDenom *bindings.CreateDenom) error {
	msgServer := tokenfactorykeeper.NewMsgServerImpl(*f)

	msgCreateDenom := tokenfactorytypes.NewMsgCreateDenom(contractAddr.String(), createDenom.Subdenom)

	// Create denom
	_, err := msgServer.CreateDenom(
		ctx,
		msgCreateDenom,
	)
	if err != nil {
		return errorsmod.Wrap(err, "creating denom")
	}
	return nil
}

// mintTokens mints tokens of a specified denom to an address.
func (m *CustomMessenger) mintTokens(ctx sdk.Context, contractAddr sdk.AccAddress, mint *bindings.MintTokens) (events []sdk.Event, data [][]byte, msgResponses [][]*codectypes.Any, err error) {
	err = PerformMint(m.TokenFactory, m.Bank, ctx, contractAddr, mint)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrap(err, "perform mint")
	}
	return nil, nil, nil, nil
}

// PerformMint used with mintTokens to validate the mint message and mint through token factory.
func PerformMint(f *tokenfactorykeeper.Keeper, _ *bankkeeper.BaseKeeper, ctx sdk.Context, contractAddr sdk.AccAddress, mint *bindings.MintTokens) error {
	rcpt, err := parseAddress(mint.MintToAddress)
	if err != nil {
		return err
	}

	coin := sdk.Coin{Denom: mint.Denom, Amount: mint.Amount}
	sdkMsg := tokenfactorytypes.NewMsgMintTo(contractAddr.String(), coin, rcpt.String())

	// Mint through token factory / message server
	msgServer := tokenfactorykeeper.NewMsgServerImpl(*f)
	_, err = msgServer.Mint(ctx, sdkMsg)
	if err != nil {
		return errorsmod.Wrap(err, "minting coins from message")
	}

	return nil
}

// changeAdmin changes the admin.
func (m *CustomMessenger) changeAdmin(ctx sdk.Context, contractAddr sdk.AccAddress, changeAdmin *bindings.ChangeAdmin) (events []sdk.Event, data [][]byte, msgResponses [][]*codectypes.Any, err error) {
	err = ChangeAdmin(m.TokenFactory, ctx, contractAddr, changeAdmin)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrap(err, "failed to change admin")
	}

	return nil, nil, nil, nil
}

// ChangeAdmin is used with changeAdmin to validate changeAdmin messages and to dispatch.
func ChangeAdmin(f *tokenfactorykeeper.Keeper, ctx sdk.Context, contractAddr sdk.AccAddress, changeAdmin *bindings.ChangeAdmin) error {
	newAdminAddr, err := parseAddress(changeAdmin.NewAdminAddress)
	if err != nil {
		return err
	}

	changeAdminMsg := tokenfactorytypes.NewMsgChangeAdmin(contractAddr.String(), changeAdmin.Denom, newAdminAddr.String())

	msgServer := tokenfactorykeeper.NewMsgServerImpl(*f)
	_, err = msgServer.ChangeAdmin(ctx, changeAdminMsg)
	if err != nil {
		return errorsmod.Wrap(err, "failed changing admin from message")
	}
	return nil
}

// burnTokens burns tokens.
func (m *CustomMessenger) burnTokens(ctx sdk.Context, contractAddr sdk.AccAddress, burn *bindings.BurnTokens) (events []sdk.Event, data [][]byte, msgResponses [][]*codectypes.Any, err error) {
	err = PerformBurn(m.TokenFactory, ctx, contractAddr, burn)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrap(err, "perform burn")
	}

	return nil, nil, nil, nil
}

// PerformBurn performs token burning after validating tokenBurn message.
func PerformBurn(f *tokenfactorykeeper.Keeper, ctx sdk.Context, contractAddr sdk.AccAddress, burn *bindings.BurnTokens) error {
	coin := sdk.Coin{Denom: burn.Denom, Amount: burn.Amount}
	sdkMsg := tokenfactorytypes.NewMsgBurnFrom(contractAddr.String(), coin, burn.BurnFromAddress)

	// Burn through token factory / message server
	msgServer := tokenfactorykeeper.NewMsgServerImpl(*f)
	_, err := msgServer.Burn(ctx, sdkMsg)
	if err != nil {
		return errorsmod.Wrap(err, "burning coins from message")
	}

	return nil
}

// createDenom forces a transfer of a tokenFactory token
func (m *CustomMessenger) forceTransfer(ctx sdk.Context, contractAddr sdk.AccAddress, forceTransfer *bindings.ForceTransfer) ([]sdk.Event, [][]byte, [][]*types.Any, error) {
	err := PerformForceTransfer(m.TokenFactory, ctx, contractAddr, forceTransfer)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrap(err, "perform force transfer")
	}
	return nil, nil, nil, nil
}

// PerformForceTransfer is used with forceTransfer to force a tokenfactory token transfer; validates the msgForceTransfer.
func PerformForceTransfer(f *tokenfactorykeeper.Keeper, ctx sdk.Context, contractAddr sdk.AccAddress, forceTransfer *bindings.ForceTransfer) error {
	msgServer := tokenfactorykeeper.NewMsgServerImpl(*f)

	msgForceTransfer := tokenfactorytypes.NewMsgForceTransfer(contractAddr.String(), sdk.NewInt64Coin(forceTransfer.Denom, forceTransfer.Amount.Int64()), forceTransfer.TransferFromAddress, forceTransfer.TransferToAddress)

	// Force Transfer
	_, err := msgServer.ForceTransfer(
		ctx,
		msgForceTransfer,
	)
	if err != nil {
		return errorsmod.Wrap(err, "forcing transfer")
	}
	return nil
}

// setDenomMetadata sets a metadata for a tokenfactory denom
func (m *CustomMessenger) setDenomMetadata(ctx sdk.Context, contractAddr sdk.AccAddress, setDenomMetadata *bindings.SetDenomMetadata) ([]sdk.Event, [][]byte, [][]*types.Any, error) {
	err := PerformSetDenomMetadata(m.TokenFactory, ctx, contractAddr, setDenomMetadata)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrap(err, "perform set denom metadata")
	}
	return nil, nil, nil, nil
}

// PerformSetDenomMetadata is used with setDenomMetadata to set a metadata for a tokenfactory denom; validates the msgSetDenomMetadata.
func PerformSetDenomMetadata(f *tokenfactorykeeper.Keeper, ctx sdk.Context, contractAddr sdk.AccAddress, setDenomMetadata *bindings.SetDenomMetadata) error {
	msgServer := tokenfactorykeeper.NewMsgServerImpl(*f)

	msgSetDenomMetadata := tokenfactorytypes.NewMsgSetDenomMetadata(contractAddr.String(), setDenomMetadata.Metadata)

	// Set denom metadata
	_, err := msgServer.SetDenomMetadata(
		ctx,
		msgSetDenomMetadata,
	)
	if err != nil {
		return errorsmod.Wrap(err, "setting denom metadata")
	}
	return nil
}

// setBeforeSendHook sets before send hook for a specified denom.
func (m *CustomMessenger) setBeforeSendHook(ctx sdk.Context, contractAddr sdk.AccAddress, set *bindings.SetBeforeSendHook) ([]sdk.Event, [][]byte, [][]*types.Any, error) {
	err := PerformSetBeforeSendHook(m.TokenFactory, ctx, contractAddr, set)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrap(err, "failed to perform set before send hook")
	}
	return nil, nil, nil, nil
}

func PerformSetBeforeSendHook(f *tokenfactorykeeper.Keeper, ctx sdk.Context, contractAddr sdk.AccAddress, set *bindings.SetBeforeSendHook) error {
	sdkMsg := tokenfactorytypes.NewMsgSetBeforeSendHook(contractAddr.String(), set.Denom, set.ContractAddr)

	// SetBeforeSendHook through token factory / message server
	msgServer := tokenfactorykeeper.NewMsgServerImpl(*f)
	_, err := msgServer.SetBeforeSendHook(ctx, sdkMsg)
	if err != nil {
		return errorsmod.Wrap(err, "set before send from message")
	}

	return nil
}

func (m *CustomMessenger) addSchedule(ctx sdk.Context, contractAddr sdk.AccAddress, addSchedule *bindings.AddSchedule) ([]sdk.Event, [][]byte, [][]*types.Any, error) {
	if !m.isAdmin(ctx, contractAddr) {
		return nil, nil, nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "only admin can add schedule")
	}

	authority := authtypes.NewModuleAddress(admintypes.ModuleName)

	msgs := make([]crontypes.MsgExecuteContract, 0, len(addSchedule.Msgs))
	for _, msg := range addSchedule.Msgs {
		msgs = append(msgs, crontypes.MsgExecuteContract{
			Contract: msg.Contract,
			Msg:      msg.Msg,
		})
	}

	_, err := m.CronMsgServer.AddSchedule(ctx, &crontypes.MsgAddSchedule{
		Authority:      authority.String(),
		Name:           addSchedule.Name,
		Period:         addSchedule.Period,
		Msgs:           msgs,
		ExecutionStage: crontypes.ExecutionStage(crontypes.ExecutionStage_value[addSchedule.ExecutionStage]),
	})
	if err != nil {
		ctx.Logger().Error("failed to addSchedule",
			"from_address", contractAddr.String(),
			"name", addSchedule.Name,
			"error", err,
		)
		return nil, nil, nil, errorsmod.Wrapf(err, "failed to add %s schedule", addSchedule.Name)
	}

	ctx.Logger().Debug("schedule added",
		"from_address", contractAddr.String(),
		"name", addSchedule.Name,
		"period", addSchedule.Period,
	)

	return nil, nil, nil, nil
}

func (m *CustomMessenger) removeSchedule(ctx sdk.Context, contractAddr sdk.AccAddress, removeSchedule *bindings.RemoveSchedule) ([]sdk.Event, [][]byte, [][]*types.Any, error) {
	params, err := m.CronQueryServer.Params(ctx, &crontypes.QueryParamsRequest{})
	if err != nil {
		ctx.Logger().Error("failed to removeSchedule", "error", err)
		return nil, nil, nil, errorsmod.Wrap(err, "failed to removeSchedule")
	}

	if !m.isAdmin(ctx, contractAddr) && contractAddr.String() != params.Params.SecurityAddress {
		return nil, nil, nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "only admin or security dao can remove schedule")
	}

	authority := authtypes.NewModuleAddress(admintypes.ModuleName)

	_, err = m.CronMsgServer.RemoveSchedule(ctx, &crontypes.MsgRemoveSchedule{
		Authority: authority.String(),
		Name:      removeSchedule.Name,
	})
	if err != nil {
		ctx.Logger().Error("failed to removeSchedule",
			"from_address", contractAddr.String(),
			"name", removeSchedule.Name,
			"error", err,
		)
		return nil, nil, nil, errorsmod.Wrapf(err, "failed to remove %s schedule", removeSchedule.Name)
	}

	ctx.Logger().Debug("schedule removed",
		"from_address", contractAddr.String(),
		"name", removeSchedule.Name,
	)
	return nil, nil, nil, nil
}

func (m *CustomMessenger) resubmitFailure(ctx sdk.Context, contractAddr sdk.AccAddress, resubmitFailure *bindings.ResubmitFailure) ([]sdk.Event, [][]byte, [][]*types.Any, error) {
	failure, err := m.ContractmanagerQueryServer.AddressFailure(ctx, &contractmanagertypes.QueryFailureRequest{
		Address:   contractAddr.String(),
		FailureId: resubmitFailure.FailureId,
	})
	if err != nil {
		return nil, nil, nil, errorsmod.Wrapf(err, "no failure with given FailureId found to resubmit")
	}

	_, err = m.ContractmanagerMsgServer.ResubmitFailure(ctx, &contractmanagertypes.MsgResubmitFailure{
		Sender:    contractAddr.String(),
		FailureId: resubmitFailure.FailureId,
	})
	if err != nil {
		ctx.Logger().Error("failed to resubmitFailure",
			"from_address", contractAddr.String(),
			"error", err,
		)
		return nil, nil, nil, errorsmod.Wrap(err, "failed to resubmitFailure")
	}

	resp := bindings.ResubmitFailureResponse{FailureId: resubmitFailure.FailureId}
	data, err := json.Marshal(&resp)
	if err != nil {
		ctx.Logger().Error("json.Marshal: failed to marshal remove resubmitFailure response to JSON",
			"from_address", contractAddr.String(),
			"error", err,
		)
		return nil, nil, nil, errorsmod.Wrap(err, "marshal json failed")
	}

	// Return failure for reverse compatibility purposes.
	// Maybe it'll be removed in the future because it was already deleted after resubmit before returning here.
	anyResp, err := types.NewAnyWithValue(failure)
	if err != nil {
		return nil, nil, nil, errorsmod.Wrapf(err, "failed to convert {%T} to Any", failure)
	}
	msgResponses := [][]*types.Any{{anyResp}}
	return nil, [][]byte{data}, msgResponses, nil
}

// GetFullDenom is a function, not method, so the message_plugin can use it
func GetFullDenom(contract string, subDenom string) (string, error) {
	// Address validation
	if _, err := parseAddress(contract); err != nil {
		return "", err
	}
	fullDenom, err := tokenfactorytypes.GetTokenDenom(contract, subDenom)
	if err != nil {
		return "", errorsmod.Wrap(err, "validate sub-denom")
	}

	return fullDenom, nil
}

// parseAddress parses address from bech32 string and verifies its format.
func parseAddress(addr string) (sdk.AccAddress, error) {
	parsed, err := sdk.AccAddressFromBech32(addr)
	if err != nil {
		return nil, errorsmod.Wrap(err, "address from bech32")
	}
	err = sdk.VerifyAddressFormat(parsed)
	if err != nil {
		return nil, errorsmod.Wrap(err, "verify address format")
	}
	return parsed, nil
}

func (m *CustomMessenger) isAdmin(ctx sdk.Context, contractAddr sdk.AccAddress) bool {
	for _, admin := range m.AdminKeeper.GetAdmins(ctx) {
		if admin == contractAddr.String() {
			return true
		}
	}

	return false
}

func (m *CustomMessenger) validateProposalQty(proposal *bindings.AdminProposal) error {
	qty := 0
	if proposal.ParamChangeProposal != nil {
		qty++
	}
	if proposal.ProposalExecuteMessage != nil {
		qty++
	}

	switch qty {
	case 1:
		return nil
	case 0:
		return fmt.Errorf("no admin proposal type is present in message")
	default:
		return fmt.Errorf("more than one admin proposal type is present in message")
	}
}

func (m *CustomMessenger) isLegacyProposal(proposal *bindings.AdminProposal) bool {
	switch {
	case proposal.ParamChangeProposal != nil:
		return true
	default:
		return false
	}
}
