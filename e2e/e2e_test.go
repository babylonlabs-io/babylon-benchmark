package e2e

import (
	"context"
	"github.com/babylonlabs-io/babylon-benchmark/config"
	"github.com/babylonlabs-io/babylon-benchmark/harness"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCreatesDelegations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5§*time.Minute)
	defer cancel()

	cfg := config.Config{
		NumPubRand:             100,
		NumMatureOutputs:       1000,
		TotalStakers:           20,
		TotalFinalityProviders: 3,
		TotalDelegations:       50,
		BabylonPath:            "",
	}

	done := make(chan error, 1)
	go func() {
		err := harness.Run(ctx, cfg)
		done <- err
	}()

	select {
	case <-ctx.Done():
		t.Fatalf("Test failed due to timeout: %v", ctx.Err())
	case err := <-done:
		require.NoError(t, err)
	}
}
