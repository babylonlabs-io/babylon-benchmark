package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	initMode     string
	snapshotPath string
	outputDir    string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "bench_babylon",
	Short: "Benchmark Babylon Nodes",
	Long:  `A CLI tool to benchmark Babylon nodes by initializing them using snapshots or data generation.`,
	Run: func(cmd *cobra.Command, args []string) {
		if initMode == "snapshot" {
			if snapshotPath == "" {
				log.Fatal("Error: --snapshot-path is required when --init-mode is snapshot")
			}
			if outputDir == "" {
				log.Fatal("Error: --output is required to specify the profiles directory")
			}
			initializeSnapshot(snapshotPath, outputDir)
		} else {
			log.Fatalf("Error: Unsupported init-mode '%s'. Currently, only 'snapshot' mode is supported.", initMode)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// Define flags
	rootCmd.Flags().StringVar(&initMode, "init-mode", "", "Initialization mode: snapshot or generate")
	rootCmd.MarkFlagRequired("init-mode")

	rootCmd.Flags().StringVar(&snapshotPath, "snapshot-path", "", "Path to the snapshot tar.gz file (required if init-mode is snapshot)")
	rootCmd.Flags().StringVar(&outputDir, "output", "", "Directory to store the profiles output")

	// Optionally, you can set default values or make flags persistent
}

// initializeSnapshot handles the snapshot initialization process
func initializeSnapshot(path, output string) {
	log.Println("Initializing Master Node with Snapshot...")

	// Verify that the snapshot file exists
	absSnapshotPath, err := filepath.Abs(path)
	if err != nil {
		log.Fatalf("Failed to get absolute path of snapshot: %v", err)
	}

	if _, err := os.Stat(absSnapshotPath); os.IsNotExist(err) {
		log.Fatalf("Snapshot file does not exist at path: %s", absSnapshotPath)
	}

	// Execute the shell script to initialize with snapshot
	initCmd := exec.Command("bash", "scripts/initialize_master_snapshot.sh", absSnapshotPath)
	initCmd.Stdout = os.Stdout
	initCmd.Stderr = os.Stderr

	err = initCmd.Run()
	if err != nil {
		log.Fatalf("Failed to initialize master node with snapshot: %v", err)
	}

	log.Println("Master Node Initialized with Snapshot.")

	// Collect the profile
	collectProfile(output)
}

// collectProfile handles the collection of the follower profile
func collectProfile(profileDir string) {
	log.Println("Collecting follower profile...")

	// Ensure the profiles directory exists
	err := os.MkdirAll(profileDir, 0755)
	if err != nil {
		log.Fatalf("Failed to create profiles directory: %v", err)
	}

	// Define source (inside Docker container) and destination paths
	source := "babylondnode1:/path/to/profile/follower_profile.json" // Update this path as needed
	destination := filepath.Join(profileDir, "follower_profile.json")

	// Execute the docker cp command to copy the profile from the container to the host
	cpCmd := exec.Command("docker", "cp", source, destination)
	cpCmd.Stdout = os.Stdout
	cpCmd.Stderr = os.Stderr

	err = cpCmd.Run()
	if err != nil {
		log.Fatalf("Failed to collect follower profile: %v", err)
	}

	log.Printf("Follower profile successfully collected at: %s\n", destination)
}
