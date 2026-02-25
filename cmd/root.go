package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "siem",
	Short: "SafePC SIEM 시스템",
	Long:  "CEP와 UEBA 서브시스템",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(cepCmd)
	rootCmd.AddCommand(uebaCmd)
}
