package prover

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/suite"

	"github.com/taikoxyz/taiko-mono/packages/taiko-client/bindings/encoding"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/bindings/metadata"
	ontakeBindings "github.com/taikoxyz/taiko-mono/packages/taiko-client/bindings/ontake"
	pacayaBindings "github.com/taikoxyz/taiko-mono/packages/taiko-client/bindings/pacaya"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/driver"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/internal/metrics"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/internal/testutils"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/pkg/jwt"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/pkg/rpc"
	"github.com/taikoxyz/taiko-mono/packages/taiko-client/proposer"
	guardianProverHeartbeater "github.com/taikoxyz/taiko-mono/packages/taiko-client/prover/guardian_prover_heartbeater"
	producer "github.com/taikoxyz/taiko-mono/packages/taiko-client/prover/proof_producer"
	proofSubmitter "github.com/taikoxyz/taiko-mono/packages/taiko-client/prover/proof_submitter"
)

type ProverTestSuite struct {
	testutils.ClientTestSuite
	p        *Prover
	cancel   context.CancelFunc
	d        *driver.Driver
	proposer *proposer.Proposer
	txmgr    *txmgr.SimpleTxManager
}

func (s *ProverTestSuite) SetupTest() {
	s.ClientTestSuite.SetupTest()

	// Init prover
	l1ProverPrivKey, err := crypto.ToECDSA(common.FromHex(os.Getenv("L1_PROVER_PRIVATE_KEY")))
	s.Nil(err)

	s.txmgr, err = txmgr.NewSimpleTxManager(
		"prover_test",
		log.Root(),
		&metrics.TxMgrMetrics,
		txmgr.CLIConfig{
			L1RPCURL:                  os.Getenv("L1_WS"),
			NumConfirmations:          0,
			SafeAbortNonceTooLowCount: txmgr.DefaultBatcherFlagValues.SafeAbortNonceTooLowCount,
			PrivateKey:                common.Bytes2Hex(crypto.FromECDSA(l1ProverPrivKey)),
			FeeLimitMultiplier:        txmgr.DefaultBatcherFlagValues.FeeLimitMultiplier,
			FeeLimitThresholdGwei:     txmgr.DefaultBatcherFlagValues.FeeLimitThresholdGwei,
			MinBaseFeeGwei:            txmgr.DefaultBatcherFlagValues.MinBaseFeeGwei,
			MinTipCapGwei:             txmgr.DefaultBatcherFlagValues.MinTipCapGwei,
			ResubmissionTimeout:       txmgr.DefaultBatcherFlagValues.ResubmissionTimeout,
			ReceiptQueryInterval:      1 * time.Second,
			NetworkTimeout:            txmgr.DefaultBatcherFlagValues.NetworkTimeout,
			TxSendTimeout:             txmgr.DefaultBatcherFlagValues.TxSendTimeout,
			TxNotInMempoolTimeout:     txmgr.DefaultBatcherFlagValues.TxNotInMempoolTimeout,
		},
	)
	s.Nil(err)

	ctx, cancel := context.WithCancel(context.Background())
	s.initProver(ctx, l1ProverPrivKey)
	s.cancel = cancel

	// Init driver
	jwtSecret, err := jwt.ParseSecretFromFile(os.Getenv("JWT_SECRET"))
	s.Nil(err)
	s.NotEmpty(jwtSecret)

	d := new(driver.Driver)
	s.Nil(d.InitFromConfig(context.Background(), &driver.Config{
		ClientConfig: &rpc.ClientConfig{
			L1Endpoint:       os.Getenv("L1_WS"),
			L2Endpoint:       os.Getenv("L2_WS"),
			L2EngineEndpoint: os.Getenv("L2_AUTH"),
			TaikoL1Address:   common.HexToAddress(os.Getenv("TAIKO_INBOX")),
			TaikoL2Address:   common.HexToAddress(os.Getenv("TAIKO_ANCHOR")),
			JwtSecret:        string(jwtSecret),
		},
		BlobServerEndpoint: s.BlobServer.URL(),
	}))
	s.d = d

	// Init proposer
	l1ProposerPrivKey, err := crypto.ToECDSA(common.FromHex(os.Getenv("L1_PROVER_PRIVATE_KEY")))
	s.Nil(err)

	prop := new(proposer.Proposer)

	s.Nil(prop.InitFromConfig(context.Background(), &proposer.Config{
		ClientConfig: &rpc.ClientConfig{
			L1Endpoint:        os.Getenv("L1_WS"),
			L2Endpoint:        os.Getenv("L2_WS"),
			L2EngineEndpoint:  os.Getenv("L2_AUTH"),
			JwtSecret:         string(jwtSecret),
			TaikoL1Address:    common.HexToAddress(os.Getenv("TAIKO_INBOX")),
			TaikoL2Address:    common.HexToAddress(os.Getenv("TAIKO_ANCHOR")),
			TaikoTokenAddress: common.HexToAddress(os.Getenv("TAIKO_TOKEN")),
		},
		L1ProposerPrivKey:          l1ProposerPrivKey,
		L2SuggestedFeeRecipient:    common.HexToAddress(os.Getenv("L2_SUGGESTED_FEE_RECIPIENT")),
		ProposeInterval:            1024 * time.Hour,
		MaxProposedTxListsPerEpoch: 1,
	}, s.txmgr, s.txmgr))

	s.proposer = prop
	s.proposer.RegisterTxMgrSelctorToBlobServer(s.BlobServer)
}

func (s *ProverTestSuite) TestName() {
	s.Equal("prover", s.p.Name())
}

func (s *ProverTestSuite) TestInitError() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l1ProverPrivKey, err := crypto.ToECDSA(common.FromHex(os.Getenv("L1_PROVER_PRIVATE_KEY")))
	s.Nil(err)

	p := new(Prover)

	s.NotNil(InitFromConfig(ctx, p, &Config{
		L1WsEndpoint:          os.Getenv("L1_WS"),
		L2WsEndpoint:          os.Getenv("L2_WS"),
		L2HttpEndpoint:        os.Getenv("L2_HTTP"),
		TaikoL1Address:        common.HexToAddress(os.Getenv("TAIKO_INBOX")),
		TaikoL2Address:        common.HexToAddress(os.Getenv("TAIKO_ANCHOR")),
		TaikoTokenAddress:     common.HexToAddress(os.Getenv("TAIKO_TOKEN")),
		L1ProverPrivKey:       l1ProverPrivKey,
		Dummy:                 true,
		ProveUnassignedBlocks: true,
		RPCTimeout:            10 * time.Minute,
		BackOffRetryInterval:  3 * time.Second,
		BackOffMaxRetries:     12,
	}, s.txmgr, s.txmgr))
}

func (s *ProverTestSuite) TestOnBlockProposed() {
	// Init prover
	l1ProverPrivKey, err := crypto.ToECDSA(common.FromHex(os.Getenv("L1_PROVER_PRIVATE_KEY")))
	s.Nil(err)
	s.p.cfg.L1ProverPrivKey = l1ProverPrivKey
	// Valid block
	m := s.ProposeAndInsertValidBlock(s.proposer, s.d.ChainSyncer().BlobSyncer())
	s.Nil(s.p.eventHandlers.blockProposedHandler.Handle(context.Background(), m, func() {}))
	req := <-s.p.proofSubmissionCh
	s.Nil(s.p.requestProofOp(req.Meta, req.Tier))
	if m.IsPacaya() {
		s.Nil(s.p.proofSubmitterPacaya.SubmitProof(context.Background(), <-s.p.proofGenerationCh))
	} else {
		s.Nil(s.p.selectSubmitter(req.Tier).SubmitProof(context.Background(), <-s.p.proofGenerationCh))
	}
}

func (s *ProverTestSuite) TestOnBlockVerifiedEmptyBlockHash() {
	s.NotPanics(func() {
		s.p.eventHandlers.blockVerifiedHandler.Handle(&ontakeBindings.TaikoL1ClientBlockVerifiedV2{
			BlockId:   common.Big1,
			BlockHash: common.Hash{},
		})
	})
}

func (s *ProverTestSuite) TestSubmitProofOp() {
	s.NotPanics(func() {
		s.p.withRetry(func() error {
			return s.p.submitProofOp(&producer.ProofResponse{
				BlockID: common.Big1,
				Meta:    &metadata.TaikoDataBlockMetadataOntake{},
				Proof:   []byte{},
				Tier:    encoding.TierOptimisticID,
				Opts:    &producer.ProofRequestOptionsOntake{},
			})
		})
	})
	s.NotPanics(func() {
		s.p.withRetry(func() error {
			return s.p.submitProofOp(&producer.ProofResponse{
				BlockID: common.Big1,
				Meta:    &metadata.TaikoDataBlockMetadataOntake{},
				Proof:   []byte{},
				Tier:    encoding.TierOptimisticID,
				Opts:    &producer.ProofRequestOptionsOntake{},
			})
		})
	})
}

func (s *ProverTestSuite) TestOnBlockVerified() {
	id := testutils.RandomHash().Big().Uint64()
	s.NotPanics(func() {
		s.p.eventHandlers.blockVerifiedHandler.Handle(&ontakeBindings.TaikoL1ClientBlockVerifiedV2{
			BlockId: testutils.RandomHash().Big(),
			Raw: types.Log{
				BlockHash:   testutils.RandomHash(),
				BlockNumber: id,
			},
		})
	})
}

func (s *ProverTestSuite) TestProveOp() {
	m := s.ProposeAndInsertValidBlock(s.proposer, s.d.ChainSyncer().BlobSyncer())

	sink1 := make(chan *pacayaBindings.TaikoInboxClientBatchesProved)
	sink2 := make(chan *ontakeBindings.TaikoL1ClientTransitionProvedV2)
	sub1, err := s.p.rpc.PacayaClients.TaikoInbox.WatchBatchesProved(nil, sink1)
	s.Nil(err)
	sub2, err := s.p.rpc.OntakeClients.TaikoL1.WatchTransitionProvedV2(nil, sink2, nil)
	s.Nil(err)
	defer func() {
		sub1.Unsubscribe()
		sub2.Unsubscribe()
		close(sink1)
		close(sink2)
	}()

	s.Nil(s.p.proveOp())
	req := <-s.p.proofSubmissionCh
	s.Nil(s.p.requestProofOp(req.Meta, req.Tier))
	if m.IsPacaya() {
		s.Nil(s.p.proofSubmitterPacaya.SubmitProof(context.Background(), <-s.p.proofGenerationCh))
	} else {
		s.Nil(s.p.selectSubmitter(req.Tier).SubmitProof(context.Background(), <-s.p.proofGenerationCh))
	}

	var (
		blockHash  common.Hash
		parentHash common.Hash
		blockID    *big.Int
	)

	select {
	case e := <-sink1:
		tran := e.Transitions[len(e.Transitions)-1]
		blockHash = common.BytesToHash(tran.BlockHash[:])
		parentHash = common.BytesToHash(tran.ParentHash[:])
		batch, err := s.p.rpc.GetBatchByID(context.Background(), new(big.Int).SetUint64(e.BatchIds[len(e.BatchIds)-1]))
		s.Nil(err)
		blockID = new(big.Int).SetUint64(batch.LastBlockId)
	case e := <-sink2:
		blockHash = common.BytesToHash(e.Tran.BlockHash[:])
		parentHash = common.BytesToHash(e.Tran.ParentHash[:])
		blockID = e.BlockId
	}

	header, err := s.p.rpc.L2.HeaderByNumber(context.Background(), blockID)
	s.Nil(err)

	s.Equal(header.Hash(), blockHash)
	s.Equal(header.ParentHash, parentHash)
}

func (s *ProverTestSuite) TestGetBlockProofStatus() {
	parent, err := s.p.rpc.L2.HeaderByNumber(context.Background(), nil)
	s.Nil(err)

	m := s.ProposeAndInsertValidBlock(s.proposer, s.d.ChainSyncer().BlobSyncer())

	// No proof submitted
	status, err := rpc.GetBlockProofStatus(
		context.Background(),
		s.p.rpc,
		m.Ontake().GetBlockID(),
		s.p.ProverAddress(),
		rpc.ZeroAddress,
	)
	s.Nil(err)
	s.False(status.IsSubmitted)

	// Valid proof submitted
	s.Nil(s.p.proveOp())
	req := <-s.p.proofSubmissionCh
	s.Nil(s.p.requestProofOp(req.Meta, req.Tier))
	s.Nil(s.p.selectSubmitter(
		m.Ontake().GetMinTier()).SubmitProof(context.Background(), <-s.p.proofGenerationCh),
	)

	status, err = rpc.GetBlockProofStatus(
		context.Background(),
		s.p.rpc,
		m.Ontake().GetBlockID(),
		s.p.ProverAddress(),
		rpc.ZeroAddress,
	)
	s.Nil(err)

	s.True(status.IsSubmitted)
	s.False(status.Invalid)
	s.Equal(parent.Hash(), status.ParentHeader.Hash())

	m = s.ProposeAndInsertValidBlock(s.proposer, s.d.ChainSyncer().BlobSyncer())
	status, err = rpc.GetBlockProofStatus(
		context.Background(),
		s.p.rpc,
		m.Ontake().GetBlockID(),
		s.p.ProverAddress(),
		rpc.ZeroAddress,
	)
	s.Nil(err)
	s.False(status.IsSubmitted)

	s.Nil(s.p.proveOp())
	req = <-s.p.proofSubmissionCh
	s.Nil(s.p.requestProofOp(req.Meta, req.Tier))

	proofWithHeader := <-s.p.proofGenerationCh
	proofWithHeader.Opts.OntakeOptions().BlockHash = testutils.RandomHash()
	s.NotNil(s.p.selectSubmitter(
		m.Ontake().GetMinTier()).SubmitProof(context.Background(), proofWithHeader),
	)
}

func (s *ProverTestSuite) TestAggregateProofsAlreadyProved() {
	batchSize := 2
	// Init batch prover
	l1ProverPrivKey, err := crypto.ToECDSA(common.FromHex(os.Getenv("L1_PROVER_PRIVATE_KEY")))
	s.Nil(err)
	decimal, err := s.RPCClient.PacayaClients.TaikoToken.Decimals(nil)
	s.Nil(err)
	batchProver := new(Prover)
	s.Nil(InitFromConfig(context.Background(), batchProver, &Config{
		L1WsEndpoint:          os.Getenv("L1_WS"),
		L2WsEndpoint:          os.Getenv("L2_WS"),
		L2HttpEndpoint:        os.Getenv("L2_HTTP"),
		TaikoL1Address:        common.HexToAddress(os.Getenv("TAIKO_INBOX")),
		TaikoL2Address:        common.HexToAddress(os.Getenv("TAIKO_ANCHOR")),
		TaikoTokenAddress:     common.HexToAddress(os.Getenv("TAIKO_TOKEN")),
		L1ProverPrivKey:       l1ProverPrivKey,
		Dummy:                 true,
		ProveUnassignedBlocks: true,
		Allowance:             new(big.Int).Exp(big.NewInt(1_000_000_100), new(big.Int).SetUint64(uint64(decimal)), nil),
		RPCTimeout:            3 * time.Second,
		BackOffRetryInterval:  3 * time.Second,
		BackOffMaxRetries:     12,
		L1NodeVersion:         "1.0.0",
		L2NodeVersion:         "0.1.0",
		SGXProofBufferSize:    uint64(batchSize),
	}, s.txmgr, s.txmgr))

	for i := 0; i < batchSize; i++ {
		_ = s.ProposeAndInsertValidBlock(s.proposer, s.d.ChainSyncer().BlobSyncer())
	}

	sink1 := make(chan *pacayaBindings.TaikoInboxClientBatchesProved, batchSize)
	sink2 := make(chan *ontakeBindings.TaikoL1ClientTransitionProvedV2, batchSize)
	sub1, err := s.p.rpc.PacayaClients.TaikoInbox.WatchBatchesProved(nil, sink1)
	s.Nil(err)
	sub2, err := s.p.rpc.OntakeClients.TaikoL1.WatchTransitionProvedV2(nil, sink2, nil)
	s.Nil(err)
	defer func() {
		sub1.Unsubscribe()
		sub2.Unsubscribe()
		close(sink1)
		close(sink2)
	}()

	s.Nil(s.p.proveOp())
	s.Nil(batchProver.proveOp())
	for i := 0; i < batchSize; i++ {
		req1 := <-s.p.proofSubmissionCh
		s.Nil(s.p.requestProofOp(req1.Meta, req1.Tier))
		req2 := <-batchProver.proofSubmissionCh
		s.Nil(batchProver.requestProofOp(req2.Meta, req2.Tier))
		s.Nil(s.p.selectSubmitter(req1.Tier).SubmitProof(context.Background(), <-s.p.proofGenerationCh))
	}
	tier := <-batchProver.aggregationNotify
	s.Nil(batchProver.aggregateOp(tier))
	s.ErrorIs(
		batchProver.selectSubmitter(tier).BatchSubmitProofs(context.Background(), <-batchProver.batchProofGenerationCh),
		proofSubmitter.ErrInvalidProof,
	)
	for i := 0; i < batchSize; i++ {
		select {
		case <-sink1:
		case <-sink2:
		}
	}
}

func (s *ProverTestSuite) TestAggregateProofs() {
	batchSize := 2
	// Init batch prover
	l1ProverPrivKey, err := crypto.ToECDSA(common.FromHex(os.Getenv("L1_PROVER_PRIVATE_KEY")))
	s.Nil(err)
	decimal, err := s.RPCClient.PacayaClients.TaikoToken.Decimals(nil)
	s.Nil(err)
	batchProver := new(Prover)
	s.Nil(InitFromConfig(context.Background(), batchProver, &Config{
		L1WsEndpoint:          os.Getenv("L1_WS"),
		L2WsEndpoint:          os.Getenv("L2_WS"),
		L2HttpEndpoint:        os.Getenv("L2_HTTP"),
		TaikoL1Address:        common.HexToAddress(os.Getenv("TAIKO_INBOX")),
		TaikoL2Address:        common.HexToAddress(os.Getenv("TAIKO_ANCHOR")),
		TaikoTokenAddress:     common.HexToAddress(os.Getenv("TAIKO_TOKEN")),
		L1ProverPrivKey:       l1ProverPrivKey,
		Dummy:                 true,
		ProveUnassignedBlocks: true,
		Allowance:             new(big.Int).Exp(big.NewInt(1_000_000_100), new(big.Int).SetUint64(uint64(decimal)), nil),
		RPCTimeout:            3 * time.Second,
		BackOffRetryInterval:  3 * time.Second,
		BackOffMaxRetries:     12,
		L1NodeVersion:         "1.0.0",
		L2NodeVersion:         "0.1.0",
		SGXProofBufferSize:    uint64(batchSize),
	}, s.txmgr, s.txmgr))

	for i := 0; i < batchSize; i++ {
		_ = s.ProposeAndInsertValidBlock(s.proposer, s.d.ChainSyncer().BlobSyncer())
	}

	sink := make(chan *ontakeBindings.TaikoL1ClientTransitionProvedV2, batchSize)
	sub, err := s.p.rpc.OntakeClients.TaikoL1.WatchTransitionProvedV2(nil, sink, nil)
	s.Nil(err)
	defer func() {
		sub.Unsubscribe()
		close(sink)
	}()

	s.Nil(batchProver.proveOp())
	for i := 0; i < batchSize; i++ {
		req := <-batchProver.proofSubmissionCh
		s.Nil(batchProver.requestProofOp(req.Meta, req.Tier))
	}
	tier := <-batchProver.aggregationNotify
	s.Nil(batchProver.aggregateOp(tier))
	s.Nil(batchProver.selectSubmitter(tier).BatchSubmitProofs(context.Background(), <-batchProver.batchProofGenerationCh))
	for i := 0; i < batchSize; i++ {
		<-sink
	}
}

func (s *ProverTestSuite) TestSetApprovalAlreadySetHigher() {
	originalAllowance, err := s.p.rpc.PacayaClients.TaikoToken.
		Allowance(&bind.CallOpts{}, s.p.ProverAddress(), s.p.cfg.TaikoL1Address)
	s.Nil(err)

	s.p.cfg.Allowance = common.Big1

	s.Nil(s.p.setApprovalAmount(context.Background(), s.p.cfg.TaikoL1Address))

	allowance, err := s.p.rpc.PacayaClients.TaikoToken.
		Allowance(&bind.CallOpts{}, s.p.ProverAddress(), s.p.cfg.TaikoL1Address)
	s.Nil(err)

	s.Equal(0, allowance.Cmp(originalAllowance))
}

func (s *ProverTestSuite) TearDownTest() {
	if s.p.ctx.Err() == nil {
		s.cancel()
	}
}

func TestProverTestSuite(t *testing.T) {
	suite.Run(t, new(ProverTestSuite))
}

func (s *ProverTestSuite) initProver(
	ctx context.Context,
	key *ecdsa.PrivateKey,
) {
	decimal, err := s.RPCClient.PacayaClients.TaikoToken.Decimals(nil)
	s.Nil(err)

	p := new(Prover)
	s.Nil(InitFromConfig(ctx, p, &Config{
		L1WsEndpoint:          os.Getenv("L1_WS"),
		L2WsEndpoint:          os.Getenv("L2_WS"),
		L2HttpEndpoint:        os.Getenv("L2_HTTP"),
		TaikoL1Address:        common.HexToAddress(os.Getenv("TAIKO_INBOX")),
		TaikoL2Address:        common.HexToAddress(os.Getenv("TAIKO_ANCHOR")),
		TaikoTokenAddress:     common.HexToAddress(os.Getenv("TAIKO_TOKEN")),
		L1ProverPrivKey:       key,
		Dummy:                 true,
		ProveUnassignedBlocks: true,
		Allowance:             new(big.Int).Exp(big.NewInt(1_000_000_100), new(big.Int).SetUint64(uint64(decimal)), nil),
		RPCTimeout:            3 * time.Second,
		BackOffRetryInterval:  3 * time.Second,
		BackOffMaxRetries:     12,
		L1NodeVersion:         "1.0.0",
		L2NodeVersion:         "0.1.0",
	}, s.txmgr, s.txmgr))

	p.guardianProverHeartbeater = guardianProverHeartbeater.New(
		key,
		p.cfg.GuardianProverHealthCheckServerEndpoint,
		p.rpc,
		p.ProverAddress(),
	)
	s.p = p
}
