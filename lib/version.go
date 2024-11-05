package lib

import (
	"fmt"
	"golang.org/x/mod/modfile"
	"os"
	"path/filepath"
)

// GetBabylonVersion returns babylond version from go.mod
func GetBabylonVersion() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error:", err)

	}

	fmt.Println("Current working directory:", dir)
	goModPath := filepath.Join("..", "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
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
