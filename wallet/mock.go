package wallet

import (
	"time"

	"github.com/dcrlabs/bchwallet/chain"
	"github.com/dcrlabs/bchwallet/waddrmgr"
	"github.com/gcash/bchd/chaincfg/chainhash"
	"github.com/gcash/bchd/wire"
	"github.com/gcash/bchutil"
)

type mockChainClient struct {
}

var _ chain.Interface = (*mockChainClient)(nil)

func (m *mockChainClient) Start() error {
	return nil
}

func (m *mockChainClient) Stop() {
}

func (m *mockChainClient) WaitForShutdown() {}

func (m *mockChainClient) GetBestBlock() (*chainhash.Hash, int32, error) {
	return nil, 0, nil
}

func (m *mockChainClient) GetBlock(*chainhash.Hash) (*wire.MsgBlock, error) {
	return nil, nil
}

func (m *mockChainClient) GetBlockHash(int64) (*chainhash.Hash, error) {
	return nil, nil
}

func (m *mockChainClient) GetBlockHeader(*chainhash.Hash) (*wire.BlockHeader,
	error) {
	return nil, nil
}

func (m *mockChainClient) IsCurrent() bool {
	return false
}

func (m *mockChainClient) FilterBlocks(*chain.FilterBlocksRequest) (
	*chain.FilterBlocksResponse, error) {
	return nil, nil
}

func (m *mockChainClient) BlockStamp() (*waddrmgr.BlockStamp, error) {
	return &waddrmgr.BlockStamp{
		Height:    500000,
		Hash:      chainhash.Hash{},
		Timestamp: time.Unix(1234, 0),
	}, nil
}

func (m *mockChainClient) SendRawTransaction(*wire.MsgTx, bool) (
	*chainhash.Hash, error) {
	return nil, nil
}

func (m *mockChainClient) Rescan(*chainhash.Hash, []bchutil.Address,
	map[wire.OutPoint]bchutil.Address) error {
	return nil
}

func (m *mockChainClient) NotifyReceived([]bchutil.Address) error {
	return nil
}

func (m *mockChainClient) NotifyBlocks() error {
	return nil
}

func (m *mockChainClient) Notifications() <-chan interface{} {
	return nil
}

func (m *mockChainClient) BackEnd() string {
	return "mock"
}
