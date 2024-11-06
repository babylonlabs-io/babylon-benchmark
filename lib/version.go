package lib

import (
	"fmt"
	"golang.org/x/mod/modfile"
	"os"
	"path/filepath"
)

// GetBabylonVersion returns babylond version from go.mod
func GetBabylonVersion() (string, error) {
	goModPaths := []string{filepath.Join("go.mod"), filepath.Join("..", "go.mod")}

	var data []byte
	for _, goModPath := range goModPaths {
		data, _ = os.ReadFile(goModPath)
		if data != nil {
			break
		}
	}

	if data == nil {
		return "", fmt.Errorf("go.mod not found")
	}

	// Parse the go.mod file
	modFile, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", err
	}

	const modName = "github.com/babylonlabs-io/babylon"
	for _, require := range modFile.Require {
		if require.Mod.Path == modName {
			return require.Mod.Version, nil
		}
	}

	return "", fmt.Errorf("module %s not found", modName)
}
