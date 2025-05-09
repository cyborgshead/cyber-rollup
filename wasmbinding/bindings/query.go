package bindings

// CyberQuery contains osmosis custom queries.
// See https://github.com/osmosis-labs/osmosis-bindings/blob/main/packages/bindings/src/query.rs
type CyberQuery struct {
	/// Given a subdenom minted by a contract via `OsmosisMsg::MintTokens`,
	/// returns the full denom as used by `BankMsg::Send`.
	FullDenom *FullDenom `json:"full_denom,omitempty"`
	/// Returns the admin of a denom, if the denom is a Token Factory denom.
	DenomAdmin *DenomAdmin `json:"denom_admin,omitempty"`
	// Returns the before send hook if it was set before
	BeforeSendHook *BeforeSendHook `json:"before_send_hook,omitempty"`
}

type FullDenom struct {
	CreatorAddr string `json:"creator_addr"`
	Subdenom    string `json:"subdenom"`
}

type DenomAdmin struct {
	Subdenom string `json:"subdenom"`
}

type DenomAdminResponse struct {
	Admin string `json:"admin"`
}

type FullDenomResponse struct {
	Denom string `json:"denom"`
}

type BeforeSendHook struct {
	Denom string `json:"denom"`
}

type BeforeSendHookResponse struct {
	ContractAddr string `json:"contract_addr"`
}
