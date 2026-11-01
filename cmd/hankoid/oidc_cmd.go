package main

import (
	"fmt"
	"os"

	"github.com/progrium/hankoid/oidc"
	"github.com/spf13/cobra"
)

func newOIDCCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "oidc", Short: "OIDC provider tools"}
	var out string
	genkey := &cobra.Command{
		Use:   "genkey",
		Short: "Generate an RSA signing key (PEM) for oidc.signing_key_file or OIDC_SIGNING_KEY",
		RunE: func(cmd *cobra.Command, args []string) error {
			pem, err := oidc.GenerateKeyPEM()
			if err != nil {
				return err
			}
			if out == "" {
				_, err = os.Stdout.Write(pem)
				return err
			}
			if err := os.WriteFile(out, pem, 0o600); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "wrote %s\n", out)
			return nil
		},
	}
	genkey.Flags().StringVarP(&out, "out", "o", "", "write to this file (mode 0600) instead of stdout")
	cmd.AddCommand(genkey)
	return cmd
}
