package main

import (
	"context"
	"encoding/json"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tendermint/tendermint/rpc/client/http"

	"mockexample/contracts"
	blobstreamXContract "mockexample/contracts/BlobstreamX.sol"
	cscContract "mockexample/contracts/CanonicalStateChain.sol"
	challenge "mockexample/contracts/Challenge.sol"
)

// Constants –
const BLOCKTIME = 12000 // Expected Time between L1 blocks in ms
const RBLOCK = 289
const POINTER = 0

// Contract addresses –
const BLOBSTREAMX = "0xf0c6429ebab2e7dc6e05dafb61128be21f13cb1e"
const CANONICALSTATECHAIN = "0x4e01c14A054F50Abfa37a1207b1aed3aB5AeCCfC"
const CHALLENGE = "0x7DB78664c44E43575fB6F96621C9Bcd3b7015C0F"

// Config –
var (
	ETHEREUM_RPC  = os.Getenv("ETHEREUM_RPC")  // defaults to https://ethereum-sepolia-rpc.publicnode.com
	CELESTIA_TRPC = os.Getenv("CELESTIA_TRPC") // defaults to http://mocha-4-consensus.mesa.newmetric.xyz:26657
)

func main() {
	if ETHEREUM_RPC == "" {
		ETHEREUM_RPC = "https://ethereum-sepolia-rpc.publicnode.com"
	}
	if CELESTIA_TRPC == "" {
		CELESTIA_TRPC = "http://mocha-4-consensus.mesa.newmetric.xyz:26657"
	}

	// 1. Setup the connection to the Ethereum and Tendermint RPC
	// and create the contract interfaces
	eth, err := ethclient.Dial(ETHEREUM_RPC)
	noErr(err, "failed to connect to Ethereum RPC")

	trpc, err := http.New(CELESTIA_TRPC, "/websocket")
	noErr(err, "failed to connect to Celestias Tendermint RPC")

	// - Check those connections
	_, err = eth.NetworkID(context.Background())
	noErr(err, "failed to get network ID from Ethereum RPC")

	_, err = trpc.Status(context.Background())
	noErr(err, "failed to get status from Celestia Tendermint RPC")

	// - Create the contract instances
	blobstreamX, err := blobstreamXContract.NewBlobstreamX(common.HexToAddress(BLOBSTREAMX), eth)
	noErr(err, "failed to create BlobstreamX instance")

	csc, err := cscContract.NewCanonicalStateChain(common.HexToAddress(CANONICALSTATECHAIN), eth)
	noErr(err, "failed to create CanonicalStateChain instance")

	challenge, err := challenge.NewChallenge(common.HexToAddress(CHALLENGE), eth)
	noErr(err, "failed to create Challenge instance")

	// 2. Get the latest rollup block
	header, err := csc.GetHeaderByNum(nil, big.NewInt(RBLOCK))
	noErr(err, "failed to get rollup header")

	// Check it contains the expected number of pointers
	if len(header.CelestiaPointers) < POINTER {
		log.Fatalf("header %d does not contain pointer %d", RBLOCK, POINTER)
	}

	// 3. Get the event where a data commitment was made to
	// blobstreamX
	pointerHeight := header.CelestiaPointers[POINTER].Height
	commitment, err := contracts.GetCommitmentEvent(BLOCKTIME, eth, challenge, blobstreamX, int64(pointerHeight))
	noErr(err, "failed to get commitment event")

	// 4. Get the proof for the data availability challenge
	pointer := header.CelestiaPointers[POINTER]
	proof, err := GetProof(trpc, &pointer, commitment)
	noErr(err, "failed to get proof")

	// 5. Print the proof
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(proof)
}

func GetProof(
	trpc *http.HTTP,
	pointer *cscContract.CanonicalStateChainCelestiaPointer,
	commitment *blobstreamXContract.BlobstreamXDataCommitmentStored,
) (*challenge.ChallengeDataAvailabilityChallengeDAProof, error) {
	ctx := context.Background()
	blockHeight := int64(pointer.Height)

	// 1. Get the block that contains the tx
	blockRes, err := trpc.Block(context.Background(), &blockHeight)
	if err != nil {
		return nil, err
	}

	// 2. Get the shares proof
	sharesProofs, err := trpc.ProveShares(ctx, pointer.Height, pointer.ShareStart.Uint64(), pointer.ShareStart.Uint64()+uint64(pointer.ShareLen))
	if err != nil {
		return nil, err
	}

	// 3. Verify the shares proof
	if !sharesProofs.VerifyProof() {
		return nil, err
	}

	// 4. Get the data root inclusion proof
	dcProof, err := trpc.DataRootInclusionProof(ctx, uint64(blockHeight), commitment.StartBlock, commitment.EndBlock)
	if err != nil {
		return nil, err
	}

	tuple := challenge.DataRootTuple{
		Height:   big.NewInt(blockHeight),
		DataRoot: *(*[32]byte)(blockRes.Block.DataHash),
	}

	sideNodes := make([][32]byte, len(dcProof.Proof.Aunts))
	for i, aunt := range dcProof.Proof.Aunts {
		sideNodes[i] = *(*[32]byte)(aunt)
	}
	wrappedProof := challenge.BinaryMerkleProof{
		SideNodes: sideNodes,
		Key:       big.NewInt(dcProof.Proof.Index),
		NumLeaves: big.NewInt(dcProof.Proof.Total),
	}

	proof := &challenge.ChallengeDataAvailabilityChallengeDAProof{
		RootNonce:     commitment.ProofNonce,
		DataRootTuple: tuple,
		Proof:         wrappedProof,
	}

	return proof, nil
}

func noErr(err error, reason ...string) {
	if err != nil {
		if len(reason) > 0 {
			log.Fatalf("%s: %v", reason[0], err)
		}
		log.Fatal(err)
	}
}
