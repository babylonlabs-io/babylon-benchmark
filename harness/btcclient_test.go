package harness

import (
	"testing"

	"github.com/babylonlabs-io/babylon-benchmark/config"
	"github.com/stretchr/testify/assert"
)

func TestNewBTCClient(t *testing.T) {
	tests := []struct {
		name    string
		config  config.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: config.Config{
				BTCRPC: "localhost:8332",
				BTCUser: "testuser",
				BTCPass: "testpass",
			},
			wantErr: false,
		},
		{
			name: "empty RPC",
			config: config.Config{
				
			},
		}
	}
}
