package app

import (
	capabilitykeeper "github.com/cosmos/ibc-go/modules/capability/keeper"
	ibckeeper "github.com/cosmos/ibc-go/v8/modules/core/keeper"

	"github.com/cosmos/cosmos-sdk/baseapp"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	rollkitstakingkeeper "github.com/rollkit/cosmos-sdk-starter/sdk/x/staking/keeper"

	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
)

func (app *CyberApp) GetIBCKeeper() *ibckeeper.Keeper {
	return app.IBCKeeper
}

func (app *CyberApp) GetScopedIBCKeeper() capabilitykeeper.ScopedKeeper {
	return app.ScopedIBCKeeper
}

func (app *CyberApp) GetBaseApp() *baseapp.BaseApp {
	return app.BaseApp
}

func (app *CyberApp) GetBankKeeper() bankkeeper.Keeper {
	return app.BankKeeper
}

func (app *CyberApp) GetStakingKeeper() rollkitstakingkeeper.Keeper {
	return app.StakingKeeper
}

func (app *CyberApp) GetAccountKeeper() authkeeper.AccountKeeper {
	return app.AccountKeeper
}

func (app *CyberApp) GetWasmKeeper() wasmkeeper.Keeper {
	return app.WasmKeeper
}
