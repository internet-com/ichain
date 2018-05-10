package app

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/ibc"
	"github.com/icheckteam/ichain/types"

	abci "github.com/tendermint/abci/types"
	crypto "github.com/tendermint/go-crypto"
	dbm "github.com/tendermint/tmlibs/db"
	"github.com/tendermint/tmlibs/log"
)

// Construct some global addrs and txs for tests.
var (
	chainID = "" // TODO

	accName = "foobart"

	priv1     = crypto.GenPrivKeyEd25519()
	addr1     = priv1.PubKey().Address()
	priv2     = crypto.GenPrivKeyEd25519()
	addr2     = priv2.PubKey().Address()
	addr3     = crypto.GenPrivKeyEd25519().PubKey().Address()
	priv4     = crypto.GenPrivKeyEd25519()
	addr4     = priv4.PubKey().Address()
	coins     = sdk.Coins{sdk.Coin{Denom: "foocoin", Amount: 10}}
	halfCoins = sdk.Coins{sdk.Coin{Denom: "foocoin", Amount: 10}}
	fee       = sdk.StdFee{
		Amount: sdk.Coins{sdk.Coin{Denom: "foocoin", Amount: 0}},
		Gas:    0,
	}

	sendMsg1 = bank.SendMsg{
		Inputs:  []bank.Input{bank.NewInput(addr1, coins)},
		Outputs: []bank.Output{bank.NewOutput(addr2, coins)},
	}

	sendMsg2 = bank.SendMsg{
		Inputs: []bank.Input{bank.NewInput(addr1, coins)},
		Outputs: []bank.Output{
			bank.NewOutput(addr2, halfCoins),
			bank.NewOutput(addr3, halfCoins),
		},
	}

	sendMsg3 = bank.SendMsg{
		Inputs: []bank.Input{
			bank.NewInput(addr1, coins),
			bank.NewInput(addr4, coins),
		},
		Outputs: []bank.Output{
			bank.NewOutput(addr2, coins),
			bank.NewOutput(addr3, coins),
		},
	}

	sendMsg4 = bank.SendMsg{
		Inputs: []bank.Input{
			bank.NewInput(addr2, coins),
		},
		Outputs: []bank.Output{
			bank.NewOutput(addr1, coins),
		},
	}
)

func loggerAndDBs() (log.Logger, map[string]dbm.DB) {
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout)).With("module", "sdk/app")
	dbs := map[string]dbm.DB{
		"main":    dbm.NewMemDB(),
		"acc":     dbm.NewMemDB(),
		"ibc":     dbm.NewMemDB(),
		"staking": dbm.NewMemDB(),
	}
	return logger, dbs
}

func newBasecoinApp() *IchainApp {
	logger, dbs := loggerAndDBs()
	return NewIchainApp(logger, dbs)
}

func setGenesisAccounts(bapp *IchainApp, accs ...auth.BaseAccount) error {
	genaccs := make([]*types.GenesisAccount, len(accs))
	for i, acc := range accs {
		genaccs[i] = types.NewGenesisAccount(&types.AppAccount{BaseAccount: acc, Name: accName})
	}

	genesisState := types.GenesisState{
		Accounts: genaccs,
	}

	stateBytes, err := json.MarshalIndent(genesisState, "", "\t")
	if err != nil {
		return err
	}

	// Initialize the chain
	vals := []abci.Validator{}
	bapp.InitChain(abci.RequestInitChain{Validators: vals, AppStateBytes: stateBytes})
	bapp.Commit()

	return nil
}

//_______________________________________________________________________

func TestMsgs(t *testing.T) {
	bapp := newBasecoinApp()

	msgs := []struct {
		msg sdk.Msg
	}{
		{sendMsg1},
	}

	for i, m := range msgs {
		// Run a CheckDeliver
		SignCheckDeliver(t, bapp, m.msg, []int64{int64(i)}, false, priv1)
	}
}

func TestGenesis(t *testing.T) {
	logger, dbs := loggerAndDBs()
	bapp := NewIchainApp(logger, dbs)

	// Construct some genesis bytes to reflect basecoin/types/AppAccount
	pk := crypto.GenPrivKeyEd25519().PubKey()
	addr := pk.Address()
	coins, err := sdk.ParseCoins("77foocoin,99barcoin")
	require.Nil(t, err)
	baseAcc := auth.BaseAccount{
		Address: addr,
		Coins:   coins,
	}
	acc := &types.AppAccount{BaseAccount: baseAcc, Name: "foobart"}

	err = setGenesisAccounts(bapp, baseAcc)
	assert.Nil(t, err)

	// A checkTx context
	ctx := bapp.BaseApp.NewContext(true, abci.Header{})
	res1 := bapp.accountMapper.GetAccount(ctx, baseAcc.Address)
	assert.Equal(t, acc, res1)

	// reload app and ensure the account is still there
	bapp = NewIchainApp(logger, dbs)
	ctx = bapp.BaseApp.NewContext(true, abci.Header{})
	res1 = bapp.accountMapper.GetAccount(ctx, baseAcc.Address)
	assert.Equal(t, acc, res1)
}

func TestSendMsgWithAccounts(t *testing.T) {
	bapp := newBasecoinApp()

	// Construct some genesis bytes to reflect basecoin/types/AppAccount
	// Give 77 foocoin to the first key
	coins, err := sdk.ParseCoins("77foocoin")
	require.Nil(t, err)
	baseAcc := auth.BaseAccount{
		Address: addr1,
		Coins:   coins,
	}

	// Construct genesis state
	err = setGenesisAccounts(bapp, baseAcc)
	assert.Nil(t, err)
	// A checkTx context (true)
	ctxCheck := bapp.BaseApp.NewContext(true, abci.Header{})
	res1 := bapp.accountMapper.GetAccount(ctxCheck, addr1)
	assert.Equal(t, baseAcc, res1.(*types.AppAccount).BaseAccount)

	// Run a CheckDeliver
	SignCheckDeliver(t, bapp, sendMsg1, []int64{0}, true, priv1)

	// Check balances
	CheckBalance(t, bapp, addr1, "67foocoin")
	CheckBalance(t, bapp, addr2, "10foocoin")

	// Delivering again should cause replay error
	SignCheckDeliver(t, bapp, sendMsg1, []int64{0}, false, priv1)

	// bumping the txnonce number without resigning should be an auth error
	tx := genTx(sendMsg1, []int64{0}, priv1)
	tx.Signatures[0].Sequence = 1
	res := bapp.Deliver(tx)

	assert.Equal(t, sdk.CodeUnauthorized, res.Code, res.Log)

	// resigning the tx with the bumped sequence should work
	SignCheckDeliver(t, bapp, sendMsg1, []int64{1}, true, priv1)
}

func TestSendMsgMultipleOut(t *testing.T) {
	bapp := newBasecoinApp()

	genCoins, err := sdk.ParseCoins("42foocoin")
	require.Nil(t, err)

	acc1 := auth.BaseAccount{
		Address: addr1,
		Coins:   genCoins,
	}

	acc2 := auth.BaseAccount{
		Address: addr2,
		Coins:   genCoins,
	}

	err = setGenesisAccounts(bapp, acc1, acc2)
	assert.Nil(t, err)

	// Simulate a Block
	SignCheckDeliver(t, bapp, sendMsg2, []int64{0}, true, priv1)

	// Check balances
	CheckBalance(t, bapp, addr1, "32foocoin")
	CheckBalance(t, bapp, addr2, "47foocoin")
	CheckBalance(t, bapp, addr3, "5foocoin")
}

func TestSengMsgMultipleInOut(t *testing.T) {
	bapp := newBasecoinApp()

	genCoins, err := sdk.ParseCoins("42foocoin")
	require.Nil(t, err)

	acc1 := auth.BaseAccount{
		Address: addr1,
		Coins:   genCoins,
	}

	acc2 := auth.BaseAccount{
		Address: addr2,
		Coins:   genCoins,
	}

	acc4 := auth.BaseAccount{
		Address: addr4,
		Coins:   genCoins,
	}

	err = setGenesisAccounts(bapp, acc1, acc2, acc4)
	assert.Nil(t, err)

	// CheckDeliver
	SignCheckDeliver(t, bapp, sendMsg3, []int64{0, 0}, true, priv1, priv4)

	// Check balances
	CheckBalance(t, bapp, addr1, "32foocoin")
	CheckBalance(t, bapp, addr4, "32foocoin")
	CheckBalance(t, bapp, addr2, "52foocoin")
	CheckBalance(t, bapp, addr3, "10foocoin")
}

func TestSendMsgDependent(t *testing.T) {
	bapp := newBasecoinApp()

	genCoins, err := sdk.ParseCoins("42foocoin")
	require.Nil(t, err)

	acc1 := auth.BaseAccount{
		Address: addr1,
		Coins:   genCoins,
	}

	err = setGenesisAccounts(bapp, acc1)
	assert.Nil(t, err)

	// CheckDeliver
	SignCheckDeliver(t, bapp, sendMsg1, []int64{0}, true, priv1)

	// Check balances
	CheckBalance(t, bapp, addr1, "32foocoin")
	CheckBalance(t, bapp, addr2, "10foocoin")

	// Simulate a Block
	SignCheckDeliver(t, bapp, sendMsg4, []int64{0}, true, priv2)

	// Check balances
	CheckBalance(t, bapp, addr1, "42foocoin")
}

func TestQuizMsg(t *testing.T) {
	bapp := newBasecoinApp()

	// Construct genesis state
	// Construct some genesis bytes to reflect basecoin/types/AppAccount
	baseAcc := auth.BaseAccount{
		Address: addr1,
	}
	acc1 := &types.AppAccount{BaseAccount: baseAcc, Name: "foobart"}

	// Construct genesis state
	genesisState := map[string]interface{}{
		"accounts": []*types.GenesisAccount{
			types.NewGenesisAccount(acc1),
		},
	}
	stateBytes, err := json.MarshalIndent(genesisState, "", "\t")
	require.Nil(t, err)

	// Initialize the chain (nil)
	vals := []abci.Validator{}
	bapp.InitChain(abci.RequestInitChain{Validators: vals, AppStateBytes: stateBytes})
	bapp.Commit()

	// A checkTx context (true)
	ctxCheck := bapp.BaseApp.NewContext(true, abci.Header{})
	res1 := bapp.accountMapper.GetAccount(ctxCheck, addr1)
	assert.Equal(t, acc1, res1)

}

func TestIBCMsgs(t *testing.T) {
	bapp := newBasecoinApp()

	sourceChain := "source-chain"
	destChain := "dest-chain"

	baseAcc := auth.BaseAccount{
		Address: addr1,
		Coins:   coins,
	}
	acc1 := &types.AppAccount{BaseAccount: baseAcc, Name: "foobart"}

	err := setGenesisAccounts(bapp, baseAcc)
	assert.Nil(t, err)
	// A checkTx context (true)
	ctxCheck := bapp.BaseApp.NewContext(true, abci.Header{})
	res1 := bapp.accountMapper.GetAccount(ctxCheck, addr1)
	assert.Equal(t, acc1, res1)

	packet := ibc.IBCPacket{
		SrcAddr:   addr1,
		DestAddr:  addr1,
		Coins:     coins,
		SrcChain:  sourceChain,
		DestChain: destChain,
	}

	transferMsg := ibc.IBCTransferMsg{
		IBCPacket: packet,
	}

	receiveMsg := ibc.IBCReceiveMsg{
		IBCPacket: packet,
		Relayer:   addr1,
		Sequence:  0,
	}

	SignCheckDeliver(t, bapp, transferMsg, []int64{0}, true, priv1)
	CheckBalance(t, bapp, addr1, "")
	SignCheckDeliver(t, bapp, transferMsg, []int64{1}, false, priv1)
	SignCheckDeliver(t, bapp, receiveMsg, []int64{2}, true, priv1)
	CheckBalance(t, bapp, addr1, "10foocoin")
	SignCheckDeliver(t, bapp, receiveMsg, []int64{3}, false, priv1)
}

func genTx(msg sdk.Msg, seq []int64, priv ...crypto.PrivKeyEd25519) sdk.StdTx {
	sigs := make([]sdk.StdSignature, len(priv))
	for i, p := range priv {
		sigs[i] = sdk.StdSignature{
			PubKey:    p.PubKey(),
			Signature: p.Sign(sdk.StdSignBytes(chainID, seq, fee, msg)),
			Sequence:  seq[i],
		}
	}

	return sdk.NewStdTx(msg, fee, sigs)

}

func SignCheckDeliver(t *testing.T, bapp *IchainApp, msg sdk.Msg, seq []int64, expPass bool, priv ...crypto.PrivKeyEd25519) {

	// Sign the tx
	tx := genTx(msg, seq, priv...)
	// Run a Check
	res := bapp.Check(tx)
	if expPass {
		require.Equal(t, sdk.CodeOK, res.Code, res.Log)
	} else {
		require.NotEqual(t, sdk.CodeOK, res.Code, res.Log)
	}

	// Simulate a Block
	bapp.BeginBlock(abci.RequestBeginBlock{})
	res = bapp.Deliver(tx)
	if expPass {
		require.Equal(t, sdk.CodeOK, res.Code, res.Log)
	} else {
		require.NotEqual(t, sdk.CodeOK, res.Code, res.Log)
	}
	bapp.EndBlock(abci.RequestEndBlock{})
	//bapp.Commit()
}

func CheckBalance(t *testing.T, bapp *IchainApp, addr sdk.Address, balExpected string) {
	ctxDeliver := bapp.BaseApp.NewContext(false, abci.Header{})
	res2 := bapp.accountMapper.GetAccount(ctxDeliver, addr)
	assert.Equal(t, balExpected, fmt.Sprintf("%v", res2.GetCoins()))
}
