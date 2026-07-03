package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// roomToFlagHelp is the shared --room flag description for connect/
// expose/send/receive — an alternative to --to for two people who
// already set up a room via `spur room create`/`spur room join`.
func roomToFlagHelp(subject string) string {
	return fmt.Sprintf("имя долговременной комнаты (см. 'spur room create'/'spur room join'), связывающей вас с %s — альтернатива --to, не нужно повторно обмениваться кодом/peer-id при каждом подключении", subject)
}

// newRoomCommand groups the two room-management subcommands under
// `spur room`, the same nesting pattern cobra encourages for a small
// family of related actions that don't fit as top-level verbs on their
// own (there's no bare "spur room" action, only "create"/"join").
func newRoomCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "room",
		Short: "Управление долговременными комнатами (постоянная привязка к конкретному собеседнику)",
	}
	cmd.AddCommand(
		newRoomCreateCommand(deps, defaults),
		newRoomJoinCommand(deps, defaults),
	)
	return cmd
}

func newRoomCreateCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		roomName     string
		identityPath = defaults.Identity
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Создать новую долговременную комнату и получить инвайт-токен для второго участника",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || roomName == "" {
				return errors.New("room create: укажите --server и --room")
			}
			result, err := deps.CreateRoom(cmd.Context(), serverAddr, roomName, identityPath, newVersionWarningPrinter(cmd))
			if err != nil {
				return err
			}
			cmd.Printf("комната %q создана.\n", roomName)
			cmd.Printf("инвайт-токен (передайте второму участнику, ему нужно указать его один раз в `spur room join`): %s\n", result.InviteToken)
			cmd.Printf("после того как второй участник присоединится, используйте --room %s вместо --to в connect/expose/send/receive.\n", roomName)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера")
	cmd.Flags().StringVar(&roomName, "room", "", "имя новой комнаты")
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")

	return cmd
}

func newRoomJoinCommand(deps ClientDependencies, defaults ClientDefaults) *cobra.Command {
	var (
		serverAddr   = defaults.Server
		roomName     string
		inviteToken  string
		identityPath = defaults.Identity
	)

	cmd := &cobra.Command{
		Use:   "join",
		Short: "Присоединиться ко второй, уже созданной комнате по инвайт-токену",
		RunE: func(cmd *cobra.Command, args []string) error {
			if serverAddr == "" || roomName == "" {
				return errors.New("room join: укажите --server и --room")
			}
			if err := deps.JoinRoom(cmd.Context(), serverAddr, roomName, inviteToken, identityPath, newVersionWarningPrinter(cmd)); err != nil {
				return err
			}
			cmd.Printf("вы присоединились к комнате %q.\n", roomName)
			cmd.Printf("теперь используйте --room %s вместо --to в connect/expose/send/receive.\n", roomName)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", serverAddr, "адрес rendezvous-сервера")
	cmd.Flags().StringVar(&roomName, "room", "", "имя комнаты")
	cmd.Flags().StringVar(&inviteToken, "invite", "", "инвайт-токен, полученный от создателя комнаты (не нужен при повторном join)")
	cmd.Flags().StringVar(&identityPath, "identity", identityPath, "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)")

	return cmd
}
