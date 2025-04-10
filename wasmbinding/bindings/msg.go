package bindings

import (
	"cosmossdk.io/math"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	paramChange "github.com/cosmos/cosmos-sdk/x/params/types/proposal"
	transferwrappertypes "github.com/neutron-org/neutron/v6/x/transfer/types"
)

type CyberMsg struct {
	// Token factory types
	/// Contracts can create denoms, namespaced under the contract's address.
	/// A contract may create any number of independent sub-denoms.
	CreateDenom *CreateDenom `json:"create_denom,omitempty"`
	/// Contracts can change the admin of a denom that they are the admin of.
	ChangeAdmin *ChangeAdmin `json:"change_admin,omitempty"`
	/// Contracts can mint native tokens for an existing factory denom
	/// that they are the admin of.
	MintTokens *MintTokens `json:"mint_tokens,omitempty"`
	/// Contracts can burn native tokens for an existing factory denom
	/// that they are the admin of.
	/// Currently, the burn from address must be the admin contract.
	BurnTokens *BurnTokens `json:"burn_tokens,omitempty"`
	/// Contracts can set before send hook for an existing factory denom
	///	that they are the admin of.
	///	Currently, the set before hook call should be performed from address that must be the admin contract.
	SetBeforeSendHook *SetBeforeSendHook `json:"set_before_send_hook,omitempty"`
	/// Force transferring of a specific denom is only allowed for the creator of the denom registered during CreateDenom.
	ForceTransfer *ForceTransfer `json:"force_transfer,omitempty"`
	/// Setting of metadata for a specific denom is only allowed for the admin of the denom.
	/// It allows the overwriting of the denom metadata in the bank module.
	SetDenomMetadata *SetDenomMetadata `json:"set_denom_metadata,omitempty"`

	// Cron types
	AddSchedule    *AddSchedule    `json:"add_schedule,omitempty"`
	RemoveSchedule *RemoveSchedule `json:"remove_schedule,omitempty"`

	// Contractmanager types
	/// A contract that has failed acknowledgement can resubmit it
	ResubmitFailure *ResubmitFailure `json:"resubmit_failure,omitempty"`

	IBCTransfer         *transferwrappertypes.MsgTransfer `json:"ibc_transfer,omitempty"`
	SubmitAdminProposal *SubmitAdminProposal              `json:"submit_admin_proposal,omitempty"`
}

// CreateDenom creates a new factory denom, of denomination:
// factory/{creating contract address}/{Subdenom}
// Subdenom can be of length at most 44 characters, in [0-9a-zA-Z./]
// The (creating contract address, subdenom) pair must be unique.
// The created denom's admin is the creating contract address,
// but this admin can be changed using the ChangeAdmin binding.
type CreateDenom struct {
	Subdenom string `json:"subdenom"`
}

// ChangeAdmin changes the admin for a factory denom.
// If the NewAdminAddress is empty, the denom has no admin.
type ChangeAdmin struct {
	Denom           string `json:"denom"`
	NewAdminAddress string `json:"new_admin_address"`
}

type MintTokens struct {
	Denom         string   `json:"denom"`
	Amount        math.Int `json:"amount"`
	MintToAddress string   `json:"mint_to_address"`
}

type BurnTokens struct {
	Denom  string   `json:"denom"`
	Amount math.Int `json:"amount"`
	// BurnFromAddress must be set to "" for now.
	BurnFromAddress string `json:"burn_from_address"`
}

// SetBeforeSendHook Allowing to assign a CosmWasm contract to call with a BeforeSend hook for a specific denom is only
// allowed for the creator of the denom registered during CreateDenom.
type SetBeforeSendHook struct {
	Denom        string `json:"denom"`
	ContractAddr string `json:"contract_addr"`
}

// SetDenomMetadata is sets the denom's bank metadata
type SetDenomMetadata struct {
	banktypes.Metadata
}

// ForceTransfer forces transferring of a specific denom is only allowed for the creator of the denom registered during CreateDenom.
type ForceTransfer struct {
	Denom               string   `json:"denom"`
	Amount              math.Int `json:"amount"`
	TransferFromAddress string   `json:"transfer_from_address"`
	TransferToAddress   string   `json:"transfer_to_address"`
}

// AddSchedule adds new schedule to the cron module
type AddSchedule struct {
	Name           string               `json:"name"`
	Period         uint64               `json:"period"`
	Msgs           []MsgExecuteContract `json:"msgs"`
	ExecutionStage string               `json:"execution_stage"`
}

// AddScheduleResponse holds response AddSchedule
type AddScheduleResponse struct{}

// RemoveSchedule removes existing schedule with given name
type RemoveSchedule struct {
	Name string `json:"name"`
}

// RemoveScheduleResponse holds response RemoveSchedule
type RemoveScheduleResponse struct{}

// MsgExecuteContract defined separate from wasmtypes since we can get away with just passing the string into bindings
type MsgExecuteContract struct {
	// Contract is the address of the smart contract
	Contract string `json:"contract,omitempty"`
	// Msg json encoded message to be passed to the contract
	Msg string `json:"msg,omitempty"`
}

type ResubmitFailure struct {
	FailureId uint64 `json:"failure_id"`
}

type ResubmitFailureResponse struct {
	FailureId uint64 `json:"failure_id"`
}

type SubmitAdminProposal struct {
	AdminProposal AdminProposal `json:"admin_proposal"`
}

type AdminProposal struct {
	ParamChangeProposal    *ParamChangeProposal    `json:"param_change_proposal,omitempty"`
	ProposalExecuteMessage *ProposalExecuteMessage `json:"proposal_execute_message,omitempty"`
}

type ParamChangeProposal struct {
	Title        string                    `json:"title"`
	Description  string                    `json:"description"`
	ParamChanges []paramChange.ParamChange `json:"param_changes"`
}

type Plan struct {
	Name   string `json:"name"`
	Height int64  `json:"height"`
	Info   string `json:"info"`
}

type ProposalExecuteMessage struct {
	Message string `json:"message,omitempty"`
}
