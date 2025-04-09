package wasmbinding

import (
	"fmt"
	"github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	icacontrollertypes "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/controller/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	ibcclienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	ibcconnectiontypes "github.com/cosmos/ibc-go/v8/modules/core/03-connection/types"
	ibcchanneltypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	crontypes "github.com/neutron-org/neutron/v6/x/cron/types"
	"sync"

	wasmvmtypes "github.com/CosmWasm/wasmvm/v2/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypesv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"

	"github.com/cosmos/gogoproto/proto"

	//tokenfactorytypes "github.com/osmosis-labs/osmosis/v28/x/tokenfactory/types"
	tokenfactorytypes "github.com/neutron-org/neutron/v6/x/tokenfactory/types"
)

// stargateResponsePools keeps whitelist and its deterministic
// response binding for stargate queries.
// CONTRACT: since results of queries go into blocks, queries being added here should always be
// deterministic or can cause non-determinism in the state machine.
//
// The query is multi-threaded so we're using a sync.Pool
// to manage the allocation and de-allocation of newly created
// pb objects.
var stargateResponsePools = make(map[string]*sync.Pool)

// Note: When adding a migration here, we should also add it to the Async ICQ params in the upgrade.
// In the future we may want to find a better way to keep these in sync

//nolint:staticcheck
func init() {
	// cosmos-sdk queries

	// auth
	setWhitelistedQuery("/cosmos.auth.v1beta1.Query/Account", &authtypes.QueryAccountResponse{})
	setWhitelistedQuery("/cosmos.auth.v1beta1.Query/Params", &authtypes.QueryParamsResponse{})
	setWhitelistedQuery("/cosmos.auth.v1beta1.Query/ModuleAccounts", &authtypes.QueryModuleAccountsResponse{})

	// bank
	setWhitelistedQuery("/cosmos.bank.v1beta1.Query/Balance", &banktypes.QueryBalanceResponse{})
	setWhitelistedQuery("/cosmos.bank.v1beta1.Query/DenomMetadata", &banktypes.QueryDenomMetadataResponse{})
	setWhitelistedQuery("/cosmos.bank.v1beta1.Query/DenomsMetadata", &banktypes.QueryDenomsMetadataResponse{})
	setWhitelistedQuery("/cosmos.bank.v1beta1.Query/Params", &banktypes.QueryParamsResponse{})
	setWhitelistedQuery("/cosmos.bank.v1beta1.Query/SupplyOf", &banktypes.QuerySupplyOfResponse{})

	// gov
	setWhitelistedQuery("/cosmos.gov.v1beta1.Query/Deposit", &govtypesv1.QueryDepositResponse{})
	setWhitelistedQuery("/cosmos.gov.v1beta1.Query/Params", &govtypesv1.QueryParamsResponse{})
	setWhitelistedQuery("/cosmos.gov.v1beta1.Query/Vote", &govtypesv1.QueryVoteResponse{})

	// distribution
	setWhitelistedQuery("/cosmos.distribution.v1beta1.Query/DelegationRewards", &types.QueryDelegationRewardsResponse{})

	// staking
	setWhitelistedQuery("/cosmos.staking.v1beta1.Query/Delegation", &stakingtypes.QueryDelegationResponse{})
	setWhitelistedQuery("/cosmos.staking.v1beta1.Query/Params", &stakingtypes.QueryParamsResponse{})
	setWhitelistedQuery("/cosmos.staking.v1beta1.Query/Validator", &stakingtypes.QueryValidatorResponse{})

	// tokenfactory
	setWhitelistedQuery("/osmosis.tokenfactory.v1beta1.Query/Params", &tokenfactorytypes.QueryParamsResponse{})
	setWhitelistedQuery("/osmosis.tokenfactory.v1beta1.Query/DenomAuthorityMetadata", &tokenfactorytypes.QueryDenomAuthorityMetadataResponse{})
	setWhitelistedQuery("/osmosis.tokenfactory.v1beta1.Query/DenomsFromCreator", &tokenfactorytypes.QueryDenomsFromCreatorResponse{})
	setWhitelistedQuery("/osmosis.tokenfactory.v1beta1.Query/BeforeSendHookAddress", &tokenfactorytypes.QueryBeforeSendHookAddressResponse{})
	setWhitelistedQuery("/osmosis.tokenfactory.v1beta1.Query/FullDenom", &tokenfactorytypes.QueryFullDenomResponse{})

	// cron
	setWhitelistedQuery("/neutron.cron.Query/Params", &crontypes.QueryParamsResponse{})

	// ibc
	setWhitelistedQuery("/ibc.core.client.v1.Query/ClientState", &ibcclienttypes.QueryClientStateResponse{})
	setWhitelistedQuery("/ibc.core.client.v1.Query/ConsensusState", &ibcclienttypes.QueryConsensusStateResponse{})
	setWhitelistedQuery("/ibc.core.connection.v1.Query/Connection", &ibcconnectiontypes.QueryConnectionResponse{})
	setWhitelistedQuery("/ibc.core.channel.v1.Query/ChannelClientState", &ibcchanneltypes.QueryChannelClientStateResponse{})

	// transfer
	setWhitelistedQuery("/ibc.applications.transfer.v1.Query/DenomTrace", &ibctransfertypes.QueryDenomTraceResponse{})
	setWhitelistedQuery("/ibc.applications.transfer.v1.Query/EscrowAddress", &ibctransfertypes.QueryEscrowAddressResponse{})

	// interchain accounts
	setWhitelistedQuery("/ibc.applications.interchain_accounts.controller.v1.Query/InterchainAccount", &icacontrollertypes.QueryInterchainAccountResponse{})
}

// IsWhitelistedQuery returns if the query is not whitelisted.
func IsWhitelistedQuery(queryPath string) error {
	_, isWhitelisted := stargateResponsePools[queryPath]
	if !isWhitelisted {
		return wasmvmtypes.UnsupportedRequest{Kind: fmt.Sprintf("'%s' path is not allowed from the contract", queryPath)}
	}
	return nil
}

// getWhitelistedQuery returns the whitelisted query at the provided path.
// If the query does not exist, or it was setup wrong by the chain, this returns an error.
// CONTRACT: must call returnStargateResponseToPool in order to avoid pointless allocs.
func getWhitelistedQuery(queryPath string) (proto.Message, error) {
	protoResponseAny, isWhitelisted := stargateResponsePools[queryPath]
	if !isWhitelisted {
		return nil, wasmvmtypes.UnsupportedRequest{Kind: fmt.Sprintf("'%s' path is not allowed from the contract", queryPath)}
	}
	protoMarshaler, ok := protoResponseAny.Get().(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to assert type to proto.Messager")
	}
	return protoMarshaler, nil
}

type protoTypeG[T any] interface {
	*T
	proto.Message
}

// setWhitelistedQuery sets the whitelisted query at the provided path.
// This method also creates a sync.Pool for the provided protoMarshaler.
// We use generics so we can properly instantiate an object that the
// queryPath expects as a response.
func setWhitelistedQuery[T any, PT protoTypeG[T]](queryPath string, _ PT) {
	stargateResponsePools[queryPath] = &sync.Pool{
		New: func() any {
			return PT(new(T))
		},
	}
}

// returnStargateResponseToPool returns the provided protoMarshaler to the appropriate pool based on it's query path.
func returnStargateResponseToPool(queryPath string, pb proto.Message) {
	stargateResponsePools[queryPath].Put(pb)
}

func GetStargateWhitelistedPaths() (keys []string) {
	// Iterate over the map and collect the keys
	keys = make([]string, 0, len(stargateResponsePools))
	for k := range stargateResponsePools {
		keys = append(keys, k)
	}
	return keys
}
