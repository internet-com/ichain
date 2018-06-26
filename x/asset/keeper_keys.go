package asset

import sdk "github.com/cosmos/cosmos-sdk/types"

// TODO remove some of these prefixes once have working multistore

//nolint
var (
	// Keys for store prefixes
	AssetKey             = []byte{0x00} // prefix for each key to an asset
	AccountAssetKey      = []byte{0x01} // prefix for each key to an account
	ProposalRecipientKey = []byte{0x02} // prefix for each key to an account a proposal
	AssetChildrenKey     = []byte{0x03} // prefix for each key to an asset parent a an asset child
)

// GetAssetKey get the key for the record with address
func GetAssetKey(assetID string) []byte {
	return append(AssetKey, []byte(assetID)...)
}

// GetAccountAssetKey get the key for an account for an asset
func GetAccountAssetKey(addr sdk.Address, assetID string) []byte {
	return append(GetAccountAssetsKey(addr), []byte(assetID)...)
}

// GetAccountAssetsKey get the key for an account for all assets
func GetAccountAssetsKey(addr sdk.Address) []byte {
	return append(AccountAssetKey, []byte(addr.String())...)
}

// GetAssetChilrenKey get the key for an asset for an asset
func GetAssetChildrenKey(parent, children string) []byte {
	return append(GetAssetChildrensKey(parent), []byte(children)...)
}

func GetAssetChildrensKey(parent string) []byte {
	return append(AssetChildrenKey, []byte(parent)...)
}
