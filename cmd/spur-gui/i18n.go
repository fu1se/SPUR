package main

// catalog holds every piece of user-facing text the GUI shows, the same
// "each layer owns its own catalog" pattern internal/adapter/cli's
// catalog.go already established for the CLI (see CLAUDE.md's "Английский
// язык интерфейса" section) — typed fields, not string keys, so a typo in
// a field name fails to compile instead of silently rendering an empty
// label.
type catalog struct {
	WindowTitle string

	TabIdentity    string
	TabPortForward string
	TabTransfer    string
	TabRooms       string
	TabMesh        string

	SettingsServer     string
	SettingsStunServer string
	SettingsSave       string
	SettingsSaved      string

	IdentitySelfID      string
	IdentityCopy        string
	IdentityCopied      string
	IdentityTestButton  string
	IdentityTestRunning string
	IdentityTestOK      string
	IdentityLoadFailed  string

	PFModeConnect     string
	PFModeExpose      string
	PFTargetTo        string
	PFTargetRoom      string
	PFPeerOrCode      string
	PFRoomName        string
	PFLocalPort       string
	PFTargetPort      string
	PFStart           string
	PFStop            string
	PFStatusIdle      string
	PFStatusEstablish string
	PFStatusRunning   string
	PFStatusStopped   string
	PFStatusFailed    string
	PFPairingCode     string

	FTModeSend       string
	FTModeReceive    string
	FTPath           string
	FTChoosePath     string
	FTChooseFolder   string
	FTDestFolder     string
	FTStart          string
	FTStop           string
	FTStatusIdle     string
	FTStatusRunning  string
	FTStatusDone     string
	FTStatusFailed   string
	FTResumeTitle    string
	FTResumeQuestion string

	RoomName        string
	RoomInviteToken string
	RoomCreate      string
	RoomJoin        string
	RoomCreated     string
	RoomJoined      string

	MeshNetworkName  string
	MeshInviteToken  string
	MeshVerbose      string
	MeshJoin         string
	MeshLeave        string
	MeshStatusIdle   string
	MeshStatusJoined string
	MeshStatusFailed string
	MeshMembers      string

	ErrorTitle string
}

var ruCatalog = catalog{
	WindowTitle: "spur",

	TabIdentity:    "Личность",
	TabPortForward: "Проброс порта",
	TabTransfer:    "Файлы",
	TabRooms:       "Комнаты",
	TabMesh:        "Mesh VPN",

	SettingsServer:     "Адрес сервера",
	SettingsStunServer: "Адрес STUN-сервера",
	SettingsSave:       "Сохранить",
	SettingsSaved:      "Настройки сохранены",

	IdentitySelfID:      "Мой peer-id",
	IdentityCopy:        "Копировать",
	IdentityCopied:      "Скопировано",
	IdentityTestButton:  "Проверить связь с сервером",
	IdentityTestRunning: "Проверка...",
	IdentityTestOK:      "Сервер видит нас как",
	IdentityLoadFailed:  "Не удалось загрузить локальную личность",

	PFModeConnect:     "connect (проброс локального порта к удалённому сервису)",
	PFModeExpose:      "expose (открыть локальный сервис для удалённой стороны)",
	PFTargetTo:        "Peer-id / код подключения",
	PFTargetRoom:      "Комната",
	PFPeerOrCode:      "peer-id или код (пусто — стать хостом и получить код)",
	PFRoomName:        "имя комнаты",
	PFLocalPort:       "Локальный порт",
	PFTargetPort:      "Целевой порт (127.0.0.1)",
	PFStart:           "Запустить",
	PFStop:            "Остановить",
	PFStatusIdle:      "Не запущено",
	PFStatusEstablish: "Устанавливаем туннель...",
	PFStatusRunning:   "Работает, локальный адрес: %s",
	PFStatusStopped:   "Остановлено",
	PFStatusFailed:    "Ошибка: %s",
	PFPairingCode:     "Код для подключения: %s",

	FTModeSend:       "Отправить",
	FTModeReceive:    "Принять",
	FTPath:           "Файл или папка",
	FTChoosePath:     "Выбрать файл",
	FTChooseFolder:   "Выбрать папку",
	FTDestFolder:     "Папка назначения",
	FTStart:          "Запустить",
	FTStop:           "Отменить",
	FTStatusIdle:     "Не запущено",
	FTStatusRunning:  "%s: %s / %s",
	FTStatusDone:     "Готово",
	FTStatusFailed:   "Ошибка: %s",
	FTResumeTitle:    "Докачка",
	FTResumeQuestion: "Найдены частично полученные файлы (%d шт., %s из %s). Докачать?",

	RoomName:        "Имя комнаты",
	RoomInviteToken: "Инвайт-токен",
	RoomCreate:      "Создать комнату",
	RoomJoin:        "Войти в комнату",
	RoomCreated:     "Комната создана, инвайт-токен: %s",
	RoomJoined:      "Успешно вошли в комнату",

	MeshNetworkName:  "Имя сети",
	MeshInviteToken:  "Инвайт-токен",
	MeshVerbose:      "Подробный лог WireGuard",
	MeshJoin:         "Войти в сеть",
	MeshLeave:        "Выйти из сети",
	MeshStatusIdle:   "Не подключено",
	MeshStatusJoined: "Подключено, CIDR: %s",
	MeshStatusFailed: "Ошибка: %s",
	MeshMembers:      "Участники: %s",

	ErrorTitle: "Ошибка",
}

var enCatalog = catalog{
	WindowTitle: "spur",

	TabIdentity:    "Identity",
	TabPortForward: "Port forward",
	TabTransfer:    "Files",
	TabRooms:       "Rooms",
	TabMesh:        "Mesh VPN",

	SettingsServer:     "Server address",
	SettingsStunServer: "STUN server address",
	SettingsSave:       "Save",
	SettingsSaved:      "Settings saved",

	IdentitySelfID:      "My peer ID",
	IdentityCopy:        "Copy",
	IdentityCopied:      "Copied",
	IdentityTestButton:  "Test server connectivity",
	IdentityTestRunning: "Testing...",
	IdentityTestOK:      "Server sees us as",
	IdentityLoadFailed:  "Failed to load local identity",

	PFModeConnect:     "connect (forward a local port to a remote service)",
	PFModeExpose:      "expose (offer a local service to the remote side)",
	PFTargetTo:        "Peer ID / pairing code",
	PFTargetRoom:      "Room",
	PFPeerOrCode:      "peer ID or code (empty — become host and get a code)",
	PFRoomName:        "room name",
	PFLocalPort:       "Local port",
	PFTargetPort:      "Target port (127.0.0.1)",
	PFStart:           "Start",
	PFStop:            "Stop",
	PFStatusIdle:      "Not running",
	PFStatusEstablish: "Establishing tunnel...",
	PFStatusRunning:   "Running, local address: %s",
	PFStatusStopped:   "Stopped",
	PFStatusFailed:    "Error: %s",
	PFPairingCode:     "Pairing code: %s",

	FTModeSend:       "Send",
	FTModeReceive:    "Receive",
	FTPath:           "File or folder",
	FTChoosePath:     "Choose file",
	FTChooseFolder:   "Choose folder",
	FTDestFolder:     "Destination folder",
	FTStart:          "Start",
	FTStop:           "Cancel",
	FTStatusIdle:     "Not running",
	FTStatusRunning:  "%s: %s / %s",
	FTStatusDone:     "Done",
	FTStatusFailed:   "Error: %s",
	FTResumeTitle:    "Resume",
	FTResumeQuestion: "Found partially received files (%d, %s of %s). Resume?",

	RoomName:        "Room name",
	RoomInviteToken: "Invite token",
	RoomCreate:      "Create room",
	RoomJoin:        "Join room",
	RoomCreated:     "Room created, invite token: %s",
	RoomJoined:      "Joined the room",

	MeshNetworkName:  "Network name",
	MeshInviteToken:  "Invite token",
	MeshVerbose:      "Verbose WireGuard log",
	MeshJoin:         "Join network",
	MeshLeave:        "Leave network",
	MeshStatusIdle:   "Not connected",
	MeshStatusJoined: "Connected, CIDR: %s",
	MeshStatusFailed: "Error: %s",
	MeshMembers:      "Members: %s",

	ErrorTitle: "Error",
}

// catalogFor picks ruCatalog or enCatalog for lang ("ru" or anything
// else, defaulting to English — same fallback direction
// infra.DetectSystemLanguage already uses).
func catalogFor(lang string) *catalog {
	if lang == "ru" {
		return &ruCatalog
	}
	return &enCatalog
}
