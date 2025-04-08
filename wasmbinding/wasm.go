package wasmbinding

import (
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	adminmodulekeeper "github.com/cosmos/admin-module/v2/x/adminmodule/keeper"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	cronkeeper "github.com/neutron-org/neutron/v6/x/cron/keeper"
	transfer "github.com/neutron-org/neutron/v6/x/transfer/keeper"

	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"

	contractmanagerkeeper "github.com/neutron-org/neutron/v6/x/contractmanager/keeper"
	tokenfactorykeeper "github.com/neutron-org/neutron/v6/x/tokenfactory/keeper"
)

func RegisterCustomPlugins(
	bank *bankkeeper.BaseKeeper,
	tokenFactory *tokenfactorykeeper.Keeper,
	cronKeeper *cronkeeper.Keeper,
	adminKeeper *adminmodulekeeper.Keeper,
	transfer transfer.KeeperTransferWrapper,
	contractmanagerKeeper *contractmanagerkeeper.Keeper,
) []wasmkeeper.Option {
	wasmQueryPlugin := NewQueryPlugin(tokenFactory)

	queryPluginOpt := wasmkeeper.WithQueryPlugins(&wasmkeeper.QueryPlugins{
		Custom: CustomQuerier(wasmQueryPlugin),
	})
	messagePluginOpt := wasmkeeper.WithMessageHandlerDecorator(
		CustomMessageDecorator(bank, tokenFactory, cronKeeper, adminKeeper, transfer, contractmanagerKeeper),
	)

	return []wasmkeeper.Option{
		queryPluginOpt,
		messagePluginOpt,
	}
}

func RegisterStargateQueries(queryRouter baseapp.GRPCQueryRouter, codec codec.Codec) []wasmkeeper.Option {
	queryPluginOpt := wasmkeeper.WithQueryPlugins(&wasmkeeper.QueryPlugins{
		Stargate: StargateQuerier(queryRouter, codec),
	})

	return []wasmkeeper.Option{
		queryPluginOpt,
	}
}
