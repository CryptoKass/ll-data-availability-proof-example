package contracts

import (
	"context"
	"fmt"
	"math/big"
	blobstreamx "mockexample/contracts/BlobstreamX.sol"
	challenge "mockexample/contracts/Challenge.sol"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
)

type commitment = *blobstreamx.BlobstreamXDataCommitmentStored

func GetCommitmentEvent(
	blockTime int64,
	eth *ethclient.Client,
	challenge *challenge.Challenge,
	bstream *blobstreamx.BlobstreamX,
	height int64,
) (commitment, error) {
	// get the block ranges to scan
	scanRanges, err := challengeWindowBlockRanges(blockTime, eth, challenge)
	if err != nil {
		return nil, fmt.Errorf("failed to get challenge window block ranges: %w", err)
	}

	// scan the ranges for the commitment
	lastCommitHeight := uint64(0)
	for _, scanRange := range scanRanges {
		if len(scanRange) != 2 {
			return nil, fmt.Errorf("invalid block range")
		}

		// get all events
		events, err := bstream.FilterDataCommitmentStored(&bind.FilterOpts{
			Context: context.Background(),
			Start:   scanRange[0],
			End:     &scanRange[1],
		}, nil, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to filter events: %w", err)
		}

		for events.Next() {
			e := events.Event
			if e.EndBlock > lastCommitHeight {
				lastCommitHeight = e.EndBlock
			}

			if int64(e.StartBlock) <= height && height < int64(e.EndBlock) {
				return e, nil
			}
		}
		if err := events.Error(); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("no commitment found for height %d (last commitment is for %d)", height, lastCommitHeight)
}

func challengeWindowBlockRanges(blockTime int64, eth *ethclient.Client, challenge *challenge.Challenge) ([][]uint64, error) {
	window, err := challenge.ChallengeWindow(nil) // seconds
	if err != nil {
		return nil, fmt.Errorf("failed to get challenge window: %w", err)
	}

	// divide window by the optimistic average block time
	// to find the number of L1 blocks we need to scan
	windowsMs := window.Mul(window, big.NewInt(1000))
	numBlocksToScan := window.Div(windowsMs, big.NewInt(blockTime))

	// get the current block number
	currentBlock, err := eth.BlockNumber(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get current block number: %w", err)
	}

	// subtract the number of blocks we need to scan from the current block
	// to find the block where the challenge window has closed
	startBlock := currentBlock - numBlocksToScan.Uint64()

	// fill array with ranges of blocks to scan
	var blockRanges [][]uint64

	blockSize := uint64(10000)
	for startBlock+blockSize < currentBlock {
		blockRanges = append(blockRanges, []uint64{startBlock, startBlock + blockSize})
		startBlock += blockSize + 1
	}
	blockRanges = append(blockRanges, []uint64{startBlock, currentBlock})

	return blockRanges, nil
}
