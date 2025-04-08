package wasmbinding

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cyborgshead/cyber-rollup/wasmbinding/bindings"
	tokenfactorykeeper "github.com/neutron-org/neutron/v6/x/tokenfactory/keeper"
)

type QueryPlugin struct {
	tokenFactoryKeeper *tokenfactorykeeper.Keeper
}

// NewQueryPlugin returns a reference to a new QueryPlugin.
func NewQueryPlugin(tfk *tokenfactorykeeper.Keeper) *QueryPlugin {
	return &QueryPlugin{
		tokenFactoryKeeper: tfk,
	}
}

// GetDenomAdmin is a query to get denom admin.
func (qp QueryPlugin) GetDenomAdmin(ctx sdk.Context, denom string) (*bindings.DenomAdminResponse, error) {
	metadata, err := qp.tokenFactoryKeeper.GetAuthorityMetadata(ctx, denom)
	if err != nil {
		return nil, fmt.Errorf("failed to get admin for denom: %s", denom)
	}

	return &bindings.DenomAdminResponse{Admin: metadata.Admin}, nil
}

// GetBeforeSendHook is a query to get denom before send hook.
func (qp QueryPlugin) GetBeforeSendHook(ctx sdk.Context, denom string) (*bindings.BeforeSendHookResponse, error) {
	contractAddr := qp.tokenFactoryKeeper.GetBeforeSendHook(ctx, denom)

	return &bindings.BeforeSendHookResponse{ContractAddr: contractAddr}, nil
}
