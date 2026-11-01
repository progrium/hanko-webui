//go:build dev

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	registrars = append(registrars, func(parent *cobra.Command) {
		parent.AddCommand(&cobra.Command{
			Use:   "dev",
			Short: "Development tools (dev builds only)",
			Run: func(cmd *cobra.Command, args []string) {
				fmt.Println("hello world")
			},
		})
	})
}
