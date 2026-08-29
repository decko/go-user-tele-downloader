package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "telecli",
	Short: "Telegram user client for automatic file downloads",
	Long: `telecli is a Telegram user client that monitors channels and automatically 
downloads files using the MTProto protocol.

It supports:
- Automatic channel monitoring
- Large file downloads (up to 4GB with Telegram Premium)
- Progress tracking and resume support
- Parallel downloads for faster speeds
- File verification with SHA256`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.telecli.yaml)")
}

func main() {
	Execute()
}
