package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newReceiveCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		stunAddr     = defaults.StunServer
		peerID       string
		identityPath = defaults.Identity
		outDir       string
	)

	cmd := &cobra.Command{
		Use:   "receive",
		Short: "Принять файл или директорию от пира, который запустил `spur send`",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || stunAddr == "" || outDir == "" {
				return errors.New("receive: укажите --server, --stun-server и --out")
			}
			err := deps.Receive(cmd.Context(), serverAddr, stunAddr, peerID, identityPath, outDir, func(selfID string) {
				cmd.Printf("свой peer-id: %s\n", selfID)
			}, newProgressPrinter(cmd.ErrOrStderr(), "приём"), newCodePrinter(cmd), newResumePrompt(cmd))
			progressDone(cmd.ErrOrStderr())
			return err
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера (control-канал)")
	cmd.Flags().StringVar(&stunAddr, "stun-server", stunAddr, "адрес STUN-эндпоинта сервера")
	cmd.Flags().StringVar(&peerID, "to", "", pairingToFlagHelp("пира, которому разрешено отправлять файлы"))
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")
	cmd.Flags().StringVar(&outDir, "out", "", "директория, куда сохранять принятые файлы")

	return cmd
}
