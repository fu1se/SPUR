package cli

// catalog holds every user-facing string this package prints or builds a
// cobra command with — one instance per Lang (ruCatalog/enCatalog below).
// Fields that take arguments are plain fmt/cobra format strings (%s, %d,
// %q, ...), used with fmt.Sprintf/cmd.Printf exactly as a literal would
// be. Grouped by the file each field is used from, in the same order as
// this package's other source files, so a change to one command's text
// only touches one contiguous block here.
type catalog struct {
	// root.go
	RootClientShort string
	RootServerShort string
	ServerListening string // %s listen, %s stun, %s db
	FlagListen      string
	FlagStunListen  string
	FlagDB          string
	FlagVerbose     string
	VersionShort    string

	// shared across several commands
	FlagServer      string // "...(control channel)" variant: connect/expose/send/receive
	FlagServerPlain string // bare variant: register/join-network/room create/room join
	FlagStunServer  string
	FlagIdentity    string
	SelfIDPrinted   string // %s peer-id
	ErrorPrefix     string
	BothToAndRoom   string // %s command name

	// whoami.go
	WhoamiShort string

	// register.go
	RegisterShort           string
	RegisterMissingServer   string
	RegisterObservedAddress string // %s address

	// connect.go
	ConnectShort        string
	ConnectMissingFlags string
	FlagLocalPort       string
	ConnectToSubject    string // passed to pairingToFlagHelp/roomToFlagHelp

	// expose.go
	ExposeShort        string
	ExposeMissingFlags string
	FlagPort           string
	ExposeToSubject    string

	// join.go
	JoinShort              string
	JoinMissingFlags       string
	FlagRendezvousCoordSrv string
	FlagNetwork            string
	FlagMeshInvite         string
	FlagJoinVerbose        string

	// join_network.go
	JoinNetworkShort         string
	JoinNetworkMissingFlags  string
	JoinNetworkPrinted       string // %s network, %s cidr
	JoinNetworkInviteToken   string // %s token
	JoinNetworkMemberPrinted string // %s peer-id, %s mesh-ip

	// room.go
	RoomParentShort        string
	RoomCreateShort        string
	RoomCreateMissingFlags string
	RoomCreatedPrinted     string // %q room name
	RoomInviteToken        string // %s token
	RoomUsageHint          string // %s room name
	RoomJoinShort          string
	RoomJoinMissingFlags   string
	RoomJoinedPrinted      string // %q room name
	FlagRoomNameNew        string
	FlagRoomName           string
	FlagRoomInvite         string
	RoomToFlagHelpFormat   string // %s subject
	ConnectRoomSubject     string
	ExposeRoomSubject      string

	// send.go
	SendShort        string
	SendMissingFlags string
	SendToSubject    string
	SendRoomSubject  string
	ProgressVerbSend string

	// receive.go
	ReceiveShort        string
	ReceiveMissingFlags string
	ReceiveToSubject    string
	ReceiveRoomSubject  string
	ProgressVerbReceive string
	FlagOutDir          string

	// pairing.go
	PairingCodeTTLHint string
	PairingToFlagHelp  string // %s subject, %s ttl
	CodePrintedLine1   string // %s code
	CodePrintedLine2   string // %s ttl

	// progress.go
	ProgressOverallNoTotal   string // %s done
	ProgressOverallWithTotal string // %s done, %s total, %.0f pct
	ProgressETASuffix        string // %s eta
	ProgressLine             string // verb, name, done, total, pct, speed, overall, eta
	ETASeconds               string // %d
	ETAMinutes               string // %d, %d
	ETAHours                 string // %d, %d

	// resume.go
	ResumeQuestion string // %d files, %s have, %s total
	ResumePrompt   string

	// versioncheck.go
	VersionMismatchWarning string // %s client, %s server

	// reconnect.go
	ReconnectNotice string // %v cause, %s delay

	// friendlyerror.go
	ExplainStunHeadline           string
	ExplainStunHint               string
	ExplainExchangeHeadline       string
	ExplainExchangeHint           string
	ExplainDialHeadline           string
	ExplainDialHint               string
	ExplainEstablishHeadline      string
	ExplainEstablishHint          string
	ExplainStreamHeadline         string
	ExplainStreamHint             string
	ExplainInviteTokenHeadline    string
	ExplainInviteTokenHint        string
	ExplainAddrInUseHeadline      string
	ExplainAddrInUseHint          string
	ExplainConnRefusedHeadline    string
	ExplainConnRefusedHint        string
	ExplainNotExistHeadline       string
	ExplainNotExistHint           string
	ExplainPermissionHeadline     string
	ExplainDeadlineHeadline       string
	ExplainEOFHeadline            string
	ExplainTechnicalDetailsPrefix string

	// lang.go
	LangShort           string
	LangCurrentAuto     string // %s lang
	LangCurrentOverride string // %s lang
	LangSetConfirm      string // %s lang
	LangAutoConfirm     string
	LangInvalidArg      string // %s arg

	// cmd/spur/main.go, cmd/spur-server/main.go (composition roots)
	CtrlCWarningClient string // %s window
	CtrlCWarningServer string // %s window
}

var ruCatalog = catalog{
	RootClientShort: "spur — прямое подключение в локальную сеть в обход NAT (клиент)",
	RootServerShort: "spur — rendezvous/signaling-сервер (control plane + STUN + relay fallback)",
	ServerListening: "control-plane слушает на %s, STUN — на %s, состояние — в %s\n",
	FlagListen:      "адрес control-канала (QUIC)",
	FlagStunListen:  "адрес STUN-эндпоинта (UDP)",
	FlagDB:          "путь к файлу состояния сервера (SQLite)",
	FlagVerbose:     "подробные (debug-уровня) логи вместо info",
	VersionShort:    "Показать версию",

	FlagServer:      "адрес rendezvous-сервера (control-канал)",
	FlagServerPlain: "адрес rendezvous-сервера",
	FlagStunServer:  "адрес STUN-эндпоинта сервера",
	FlagIdentity:    "путь к файлу идентичности (по умолчанию — в конфиг-директории пользователя)",
	SelfIDPrinted:   "свой peer-id: %s\n",
	ErrorPrefix:     "Ошибка:",
	BothToAndRoom:   "%s: укажите либо --to, либо --room, не оба сразу",

	WhoamiShort: "Показать свой peer-id (без обращения к сети)",

	RegisterShort:           "Зарегистрироваться на rendezvous-сервере и показать наблюдаемый им адрес",
	RegisterMissingServer:   "register: укажите --server",
	RegisterObservedAddress: "observed-address: %s\n",

	ConnectShort:        "Пробросить локальный порт на сервис, открытый пиром через `spur expose`",
	ConnectMissingFlags: "connect: укажите --server, --stun-server и --local-port",
	FlagLocalPort:       "локальный порт для прослушивания",
	ConnectToSubject:    "пира, чей сервис пробрасываем",

	ExposeShort:        "Открыть локальный сервис указанному пиру (port-forward режим)",
	ExposeMissingFlags: "expose: укажите --server, --stun-server и --port",
	FlagPort:           "локальный порт сервиса, который открываем",
	ExposeToSubject:    "пира, которому разрешено подключаться",

	JoinShort:              "Присоединиться к mesh-сети (полноценный доступ в локальную сеть через TUN)",
	JoinMissingFlags:       "join: укажите --server, --stun-server и --network",
	FlagRendezvousCoordSrv: "адрес rendezvous/coordination-сервера",
	FlagNetwork:            "имя mesh-сети",
	FlagMeshInvite:         "инвайт-токен сети (не нужен при создании новой сети или повторном join)",
	FlagJoinVerbose:        "подробный лог WireGuard-устройства (handshake, добавление/удаление пиров) — по умолчанию только ошибки",

	JoinNetworkShort:         "Присоединиться к mesh-сети на сервере и показать её участников (без TUN)",
	JoinNetworkMissingFlags:  "join-network: укажите --server и --network",
	JoinNetworkPrinted:       "сеть: %s, cidr: %s\n",
	JoinNetworkInviteToken:   "инвайт-токен (передайте тем, кто будет присоединяться): %s\n",
	JoinNetworkMemberPrinted: "  участник: %s  mesh-ip: %s\n",

	RoomParentShort:        "Управление долговременными комнатами (постоянная привязка к конкретному собеседнику)",
	RoomCreateShort:        "Создать новую долговременную комнату и получить инвайт-токен для второго участника",
	RoomCreateMissingFlags: "room create: укажите --server и --room",
	RoomCreatedPrinted:     "комната %q создана.\n",
	RoomInviteToken:        "инвайт-токен (передайте второму участнику, ему нужно указать его один раз в `spur room join`): %s\n",
	RoomUsageHint:          "после того как второй участник присоединится, используйте --room %s вместо --to в connect/expose/send/receive.\n",
	RoomJoinShort:          "Присоединиться к уже созданной комнате по инвайт-токену",
	RoomJoinMissingFlags:   "room join: укажите --server и --room",
	RoomJoinedPrinted:      "вы присоединились к комнате %q.\n",
	FlagRoomNameNew:        "имя новой комнаты",
	FlagRoomName:           "имя комнаты",
	FlagRoomInvite:         "инвайт-токен, полученный от создателя комнаты (не нужен при повторном join)",
	RoomToFlagHelpFormat:   "имя долговременной комнаты (см. 'spur room create'/'spur room join'), связывающей вас с %s — альтернатива --to, не нужно повторно обмениваться кодом/peer-id при каждом подключении",
	ConnectRoomSubject:     "пиром, чей сервис пробрасываем",
	ExposeRoomSubject:      "пиром, которому разрешено подключаться",

	SendShort:        "Отправить файл или директорию пиру, который запустил `spur receive`",
	SendMissingFlags: "send: укажите --server и --stun-server",
	SendToSubject:    "пира, который примет файл/директорию",
	SendRoomSubject:  "пиром, который примет файл/директорию",
	ProgressVerbSend: "отправка",

	ReceiveShort:        "Принять файл или директорию от пира, который запустил `spur send`",
	ReceiveMissingFlags: "receive: укажите --server, --stun-server и --out",
	ReceiveToSubject:    "пира, которому разрешено отправлять файлы",
	ReceiveRoomSubject:  "пиром, которому разрешено отправлять файлы",
	ProgressVerbReceive: "приём",
	FlagOutDir:          "директория, куда сохранять принятые файлы",

	PairingCodeTTLHint: "10 минут",
	PairingToFlagHelp:  "идентификатор или код %s; не указан — сгенерировать свой код и ждать подключения (см. 'spur whoami' для постоянного ID, код — одноразовый на %s)",
	CodePrintedLine1:   "Код для подключения: %s\n",
	CodePrintedLine2:   "Сообщите его собеседнику — он должен указать этот код в --to. Ждём подключения (до %s)...\n",

	ProgressOverallNoTotal:   "всего: %s",
	ProgressOverallWithTotal: "всего: %s/%s (%.0f%%)",
	ProgressETASuffix:        " — осталось: %s",
	ProgressLine:             "\r\033[K%s %s: %s/%s (%.0f%%) — %s/с — %s%s",
	ETASeconds:               "~%dс",
	ETAMinutes:               "~%dм %02dс",
	ETAHours:                 "~%dч %02dм",

	ResumeQuestion: "Обнаружена незавершённая передача: %d файл(ов), уже получено %s из %s.\n",
	ResumePrompt:   "Продолжить с того места, где остановились? [Y/n] ",

	VersionMismatchWarning: "Внимание: версия клиента (%s) отличается от версии сервера (%s) — некоторые функции могут работать некорректно. Обновите обе стороны до одной версии, если возникнут проблемы.\n",

	ReconnectNotice: "Соединение потеряно (%v). Переподключение через %s...\n",

	ExplainStunHeadline:           "Не удалось связаться со STUN-сервером (--stun-server).",
	ExplainStunHint:               "Проверьте: адрес и порт указаны верно; порт открыт по UDP и на этой машине, и на сервере (файрвол/security group у облачного провайдера часто блокирует UDP по умолчанию, даже если TCP открыт).",
	ExplainExchangeHeadline:       "Второй участник не ответил вовремя (или указан не тот собеседник).",
	ExplainExchangeHint:           "Убедитесь, что второй участник запустил соответствующую команду (например `spur receive`) в течение примерно минуты после вас и указал в --to именно ваш ТЕКУЩИЙ peer-id — сверьте свежим `spur whoami`, не значением из старой истории команд.",
	ExplainDialHeadline:           "Не удалось подключиться к серверу (--server).",
	ExplainDialHint:               "Проверьте: spur-server запущен и слушает; адрес/порт указаны верно; порт открыт по UDP снаружи.",
	ExplainEstablishHeadline:      "Не удалось установить канал — ни напрямую (P2P), ни через relay-сервер.",
	ExplainEstablishHint:          "Оба способа не сработали одновременно — обычно значит, что у одной из сторон нестабильное соединение или порт сервера недоступен по UDP. Проверьте связь: ping/mtr до сервера, и что оба порта сервера (--listen и --stun-listen) открыты снаружи.",
	ExplainStreamHeadline:         "Соединение оборвалось во время передачи данных.",
	ExplainStreamHint:             "Файл мог дойти не полностью — запустите передачу заново.",
	ExplainInviteTokenHeadline:    "Неверный или отсутствующий инвайт-токен.",
	ExplainInviteTokenHint:        "Токен нужен только для входа в УЖЕ существующую сеть — уточните его у того, кто её создавал (он печатается при первом `spur join`/`spur join-network` для этой сети).",
	ExplainAddrInUseHeadline:      "Порт уже занят другим процессом.",
	ExplainAddrInUseHint:          "Проверьте `ss -ulpn | grep <порт>` — возможно, где-то уже работает второй spur-server/spur, или порт совпадает с другим сервисом. Частая ошибка: --listen и --stun-listen указаны одинаковыми — это два разных порта.",
	ExplainConnRefusedHeadline:    "Сервер отверг подключение.",
	ExplainConnRefusedHint:        "Проверьте, что spur-server запущен на этом адресе и порту.",
	ExplainNotExistHeadline:       "Файл или директория не найдены.",
	ExplainNotExistHint:           "Проверьте путь — частая причина: относительный путь (`./mnt/...`) интерпретируется от текущей директории, а не от корня диска (`/mnt/...`).",
	ExplainPermissionHeadline:     "Недостаточно прав доступа.",
	ExplainDeadlineHeadline:       "Истекло время ожидания.",
	ExplainEOFHeadline:            "Соединение закрылось раньше, чем ожидалось.",
	ExplainTechnicalDetailsPrefix: "\n  Технические детали: ",

	LangShort:           "Показать или изменить язык интерфейса",
	LangCurrentAuto:     "текущий язык: %s (определён по системной локали; чтобы задать вручную — `spur lang ru` или `spur lang en`)\n",
	LangCurrentOverride: "текущий язык: %s (задан вручную; чтобы вернуться к системной локали — `spur lang auto`)\n",
	LangSetConfirm:      "язык интерфейса установлен: %s\n",
	LangAutoConfirm:     "язык интерфейса снова определяется по системной локали.\n",
	LangInvalidArg:      "lang: неизвестный язык %q, ожидается ru, en или auto",

	CtrlCWarningClient: "\nПолучен Ctrl+C. Нажмите ещё раз в течение %s, чтобы прервать.\n",
	CtrlCWarningServer: "\nПолучен Ctrl+C. Нажмите ещё раз в течение %s, чтобы остановить сервер.\n",
}

var enCatalog = catalog{
	RootClientShort: "spur — direct connection into a local network across NAT (client)",
	RootServerShort: "spur — rendezvous/signaling server (control plane + STUN + relay fallback)",
	ServerListening: "control plane listening on %s, STUN on %s, state in %s\n",
	FlagListen:      "control channel address (QUIC)",
	FlagStunListen:  "STUN endpoint address (UDP)",
	FlagDB:          "path to the server state file (SQLite)",
	FlagVerbose:     "verbose (debug-level) logs instead of info",
	VersionShort:    "Show the build version",

	FlagServer:      "rendezvous server address (control channel)",
	FlagServerPlain: "rendezvous server address",
	FlagStunServer:  "server's STUN endpoint address",
	FlagIdentity:    "path to the identity file (defaults to the user's config directory)",
	SelfIDPrinted:   "own peer-id: %s\n",
	ErrorPrefix:     "Error:",
	BothToAndRoom:   "%s: specify either --to or --room, not both",

	WhoamiShort: "Show your own peer-id (no network access)",

	RegisterShort:           "Register with the rendezvous server and show the observed address",
	RegisterMissingServer:   "register: specify --server",
	RegisterObservedAddress: "observed-address: %s\n",

	ConnectShort:        "Forward a local port to a service the peer exposed via `spur expose`",
	ConnectMissingFlags: "connect: specify --server, --stun-server and --local-port",
	FlagLocalPort:       "local port to listen on",
	ConnectToSubject:    "the peer whose service is being forwarded",

	ExposeShort:        "Expose a local service to the given peer (port-forward mode)",
	ExposeMissingFlags: "expose: specify --server, --stun-server and --port",
	FlagPort:           "local port of the service being exposed",
	ExposeToSubject:    "the peer allowed to connect",

	JoinShort:              "Join a mesh network (full access to the local network via TUN)",
	JoinMissingFlags:       "join: specify --server, --stun-server and --network",
	FlagRendezvousCoordSrv: "rendezvous/coordination server address",
	FlagNetwork:            "mesh network name",
	FlagMeshInvite:         "network invite token (not needed to create a new network or rejoin)",
	FlagJoinVerbose:        "verbose WireGuard device logging (handshakes, peer add/remove) — errors only by default",

	JoinNetworkShort:         "Join a mesh network on the server and show its members (no TUN)",
	JoinNetworkMissingFlags:  "join-network: specify --server and --network",
	JoinNetworkPrinted:       "network: %s, cidr: %s\n",
	JoinNetworkInviteToken:   "invite token (share with whoever should join next): %s\n",
	JoinNetworkMemberPrinted: "  member: %s  mesh-ip: %s\n",

	RoomParentShort:        "Manage long-lived rooms (a permanent pairing with a specific counterpart)",
	RoomCreateShort:        "Create a new long-lived room and get an invite token for the second participant",
	RoomCreateMissingFlags: "room create: specify --server and --room",
	RoomCreatedPrinted:     "room %q created.\n",
	RoomInviteToken:        "invite token (share with the second participant, who presents it once to `spur room join`): %s\n",
	RoomUsageHint:          "once the second participant has joined, use --room %s instead of --to in connect/expose/send/receive.\n",
	RoomJoinShort:          "Join an already-created room using its invite token",
	RoomJoinMissingFlags:   "room join: specify --server and --room",
	RoomJoinedPrinted:      "you joined room %q.\n",
	FlagRoomNameNew:        "name of the new room",
	FlagRoomName:           "room name",
	FlagRoomInvite:         "invite token from the room's creator (not needed to rejoin)",
	RoomToFlagHelpFormat:   "name of a long-lived room (see 'spur room create'/'spur room join') linking you to %s — an alternative to --to, no need to re-exchange a code/peer-id every time",
	ConnectRoomSubject:     "the peer whose service is being forwarded",
	ExposeRoomSubject:      "the peer allowed to connect",

	SendShort:        "Send a file or directory to a peer running `spur receive`",
	SendMissingFlags: "send: specify --server and --stun-server",
	SendToSubject:    "the peer who will receive the file/directory",
	SendRoomSubject:  "the peer who will receive the file/directory",
	ProgressVerbSend: "sending",

	ReceiveShort:        "Receive a file or directory from a peer running `spur send`",
	ReceiveMissingFlags: "receive: specify --server, --stun-server and --out",
	ReceiveToSubject:    "the peer allowed to send files",
	ReceiveRoomSubject:  "the peer allowed to send files",
	ProgressVerbReceive: "receiving",
	FlagOutDir:          "directory to save received files into",

	PairingCodeTTLHint: "10 minutes",
	PairingToFlagHelp:  "identifier or code of %s; leave unset to generate your own code and wait for a connection (see 'spur whoami' for a permanent ID — a code is single-use, valid for %s)",
	CodePrintedLine1:   "Connection code: %s\n",
	CodePrintedLine2:   "Share it with your counterpart — they should pass it as --to. Waiting for a connection (up to %s)...\n",

	ProgressOverallNoTotal:   "total: %s",
	ProgressOverallWithTotal: "total: %s/%s (%.0f%%)",
	ProgressETASuffix:        " — remaining: %s",
	ProgressLine:             "\r\033[K%s %s: %s/%s (%.0f%%) — %s/s — %s%s",
	ETASeconds:               "~%ds",
	ETAMinutes:               "~%dm %02ds",
	ETAHours:                 "~%dh %02dm",

	ResumeQuestion: "Found an incomplete transfer: %d file(s), already have %s of %s.\n",
	ResumePrompt:   "Resume where it left off? [Y/n] ",

	VersionMismatchWarning: "Warning: client version (%s) differs from server version (%s) — some functionality may not work correctly. Update both sides to the same version if you run into trouble.\n",

	ReconnectNotice: "Connection lost (%v). Reconnecting in %s...\n",

	ExplainStunHeadline:           "Couldn't reach the STUN server (--stun-server).",
	ExplainStunHint:               "Check: the address and port are correct; the port is open over UDP both on this machine and on the server (a cloud provider's firewall/security group often blocks UDP by default even when TCP is open).",
	ExplainExchangeHeadline:       "The other participant didn't respond in time (or the wrong counterpart was specified).",
	ExplainExchangeHint:           "Make sure the other participant ran the matching command (e.g. `spur receive`) within about a minute of you, and specified your CURRENT peer-id in --to — check against a fresh `spur whoami`, not a value from old command history.",
	ExplainDialHeadline:           "Couldn't connect to the server (--server).",
	ExplainDialHint:               "Check: spur-server is running and listening; the address/port are correct; the port is open over UDP from outside.",
	ExplainEstablishHeadline:      "Couldn't establish a channel — neither directly (P2P) nor via the relay server.",
	ExplainEstablishHint:          "Both methods failed at once — usually means one side has an unstable connection, or the server's port isn't reachable over UDP. Check connectivity: ping/mtr to the server, and that both server ports (--listen and --stun-listen) are open from outside.",
	ExplainStreamHeadline:         "The connection dropped during data transfer.",
	ExplainStreamHint:             "The file may not have arrived in full — restart the transfer.",
	ExplainInviteTokenHeadline:    "Invalid or missing invite token.",
	ExplainInviteTokenHint:        "A token is only needed to join an ALREADY existing network — ask whoever created it (it's printed on the first `spur join`/`spur join-network` for that network).",
	ExplainAddrInUseHeadline:      "The port is already in use by another process.",
	ExplainAddrInUseHint:          "Check `ss -ulpn | grep <port>` — maybe another spur-server/spur is already running, or the port collides with another service. A common mistake: --listen and --stun-listen set to the same value — these are two different ports.",
	ExplainConnRefusedHeadline:    "The server refused the connection.",
	ExplainConnRefusedHint:        "Check that spur-server is running on this address and port.",
	ExplainNotExistHeadline:       "File or directory not found.",
	ExplainNotExistHint:           "Check the path — a common cause: a relative path (`./mnt/...`) is interpreted from the current directory, not the filesystem root (`/mnt/...`).",
	ExplainPermissionHeadline:     "Insufficient permissions.",
	ExplainDeadlineHeadline:       "Timed out.",
	ExplainEOFHeadline:            "The connection closed earlier than expected.",
	ExplainTechnicalDetailsPrefix: "\n  Technical details: ",

	LangShort:           "Show or change the UI language",
	LangCurrentAuto:     "current language: %s (detected from the system locale; to set it manually, use `spur lang ru` or `spur lang en`)\n",
	LangCurrentOverride: "current language: %s (set manually; to go back to the system locale, use `spur lang auto`)\n",
	LangSetConfirm:      "UI language set to: %s\n",
	LangAutoConfirm:     "UI language will be detected from the system locale again.\n",
	LangInvalidArg:      "lang: unknown language %q, expected ru, en, or auto",

	CtrlCWarningClient: "\nCtrl+C received. Press it again within %s to interrupt.\n",
	CtrlCWarningServer: "\nCtrl+C received. Press it again within %s to stop the server.\n",
}

// msg returns the active catalog, based on currentLang (see i18n.go).
func msg() *catalog {
	if currentLang == LangEN {
		return &enCatalog
	}
	return &ruCatalog
}
