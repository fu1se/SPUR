package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newServerCommand() *cobra.Command {
	var listenAddr string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Запустить rendezvous/signaling-сервер (control plane + relay fallback)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = listenAddr
			return errors.New("server: не реализовано (Фаза 2)")
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", ":4443", "адрес control-канала (QUIC)")

	return cmd
}
