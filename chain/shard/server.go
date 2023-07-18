package shard

import (
	"context"
	"crypto/tls"
	"fmt"
	"google.golang.org/grpc/credentials"
	"log"
	"net"
	"os"
	"sync"

	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shard "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"google.golang.org/grpc"

	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

var (
	Name                              = "shard_handler_server"
	_    shardgrpc.ShardHandlerServer = &Server{}
)

type Server struct {
	moduleAddr sdk.AccAddress
	lock       sync.Mutex
	msgQueue   []types.SubmitBatchRequest
	serverOpts []grpc.ServerOption
}

func NewShardServer(opts ...Option) *Server {
	addr := authtypes.NewModuleAddress(Name)
	s := &Server{
		moduleAddr: addr,
		lock:       sync.Mutex{},
		msgQueue:   nil,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func loadCredentials(certPath, keyPath string) (credentials.TransportCredentials, error) {
	// Load server's certificate and private key
	sc, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	sk, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	serverCert, err := tls.X509KeyPair(sc, sk)
	if err != nil {
		return nil, err
	}

	// Create the credentials and return it
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert,
	}

	return credentials.NewTLS(config), nil
}

// Serve serves the application in a new go routine. The routine panics if serve fails.
func (s *Server) Serve(listenAddr string) {
	grpcServer := grpc.NewServer(s.serverOpts...)
	shardgrpc.RegisterShardHandlerServer(grpcServer, s)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		err = grpcServer.Serve(listener)
		if err != nil {
			panic(err)
		}
	}()
}

// FlushMessages first copies the transactions in the queue, then clears the queue and returns the copy.
func (s *Server) FlushMessages() []types.SubmitBatchRequest {
	// no-op if we have nothing
	if len(s.msgQueue) == 0 {
		return nil
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	// copy the transactions
	msgs := make([]types.SubmitBatchRequest, len(s.msgQueue))
	copy(msgs, s.msgQueue)

	// clear the queue
	s.msgQueue = s.msgQueue[:0]
	return msgs
}

// SubmitShardBatch appends the shard tx submissions to the queue IFF they pass validation.
func (s *Server) SubmitShardBatch(_ context.Context, req *shard.SubmitShardBatchRequest) (
	*shard.SubmitShardBatchResponse, error) {
	sbr := types.SubmitBatchRequest{
		Sender: s.moduleAddr.String(),
		TransactionBatch: &types.TransactionBatch{
			Namespace: req.Namespace,
			Tick:      req.TickId,
			Batch:     req.Batch,
		},
	}
	if err := sbr.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("cannot submit batch to blockchain. invalid submission: %w", err)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	s.msgQueue = append(s.msgQueue, sbr)
	return &shard.SubmitShardBatchResponse{}, nil
}