package lib

import (
	"fmt"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cometbft/cometbft/crypto/merkle"
	"github.com/lightningnetwork/lnd/kvdb"
	"sync"
)

func GetConfig(dbPath string) *kvdb.BoltBackendConfig {
	return &kvdb.BoltBackendConfig{
		DBPath:            dbPath,
		DBFileName:        "benchmark-data.db",
		NoFreelistSync:    true,
		AutoCompact:       false,
		AutoCompactMinAge: kvdb.DefaultBoltAutoCompactMinAge,
		DBTimeout:         kvdb.DefaultDBTimeout,
	}
}

func NewBackend(dbPath string) (kvdb.Backend, error) {
	return kvdb.GetBoltBackend(GetConfig(dbPath))
}

// PubRandProofStore In-memory store for public randomness proofs
type PubRandProofStore struct {
	mu         sync.RWMutex
	proofStore map[string][]byte
}

// NewPubRandProofStore returns a new in-memory store
func NewPubRandProofStore() *PubRandProofStore {
	return &PubRandProofStore{
		proofStore: make(map[string][]byte),
	}
}

// AddPubRandProofList adds a list of pubRand and proofs to the in-memory store
func (s *PubRandProofStore) AddPubRandProofList(
	pubRandList []*btcec.FieldVal,
	proofList []*merkle.Proof,
) error {
	if len(pubRandList) != len(proofList) {
		return fmt.Errorf("the number of public randomness values is not the same as the number of proofs")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range pubRandList {
		pubRandBytes := pubRandList[i].Bytes()
		proofBytes, err := proofList[i].ToProto().Marshal()
		if err != nil {
			return fmt.Errorf("invalid proof: %w", err)
		}

		pubRandKey := string(pubRandBytes[:])
		// Skip if already committed
		if _, exists := s.proofStore[pubRandKey]; exists {
			continue
		}

		// Add to the in-memory store
		s.proofStore[pubRandKey] = proofBytes
	}

	return nil
}

// GetPubRandProof retrieves a proof for the given pubRand from the in-memory store
func (s *PubRandProofStore) GetPubRandProof(pubRand *btcec.FieldVal) ([]byte, error) {
	pubRandBytes := pubRand.Bytes()
	pubRandKey := string(pubRandBytes[:])

	s.mu.RLock()
	defer s.mu.RUnlock()

	proofBytes, exists := s.proofStore[pubRandKey]
	if !exists {
		return nil, fmt.Errorf("pubRand proof not found")
	}

	return proofBytes, nil
}

// GetPubRandProofList retrieves proofs for a list of pubRand values from the in-memory store
func (s *PubRandProofStore) GetPubRandProofList(pubRandList []*btcec.FieldVal) ([][]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var proofBytesList [][]byte
	for _, pubRand := range pubRandList {
		pubRandBytes := pubRand.Bytes()
		pubRandKey := string(pubRandBytes[:])

		proofBytes, exists := s.proofStore[pubRandKey]
		if !exists {
			return nil, fmt.Errorf("pubRand proof not found for one or more pubRands")
		}

		proofBytesList = append(proofBytesList, proofBytes)
	}

	return proofBytesList, nil
}
