package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newSendCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		identityPath = defaults.Identity
	)

	cmd := &cobra.Command{
		Use:   "send <path>",
		Short: "Отправить файл или директорию пиру, который запустил `spur receive`",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" {
				return errors.New("send: укажите --server и --stun-server")
			}
			err := deps.Send(cmd.Context(), serverAddr, stunAddr, peerID, identityPath, args[0], func(selfID string) {
				cmd.Printf("свой peer-id: %s\n", selfID)
			}, newProgressPrinter(cmd.ErrOrStderr(), "отправка"), newCodePrinter(cmd))
			progressDone(cmd.ErrOrStderr())
			return err
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера (control-канал)")
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, "адрес STUN-эндпоинта сервера")
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp("пира, который примет файл/директорию"))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")

	return cmd
}
