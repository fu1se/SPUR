package cli

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
)

// Explain turns one of this project's own wrapped errors into a short,
// actionable message for a human, when it recognizes the failure —
// falling back to the original error's message otherwise, so nothing is
// ever hidden.
//
// Every error surfaced through the composition roots (cmd/spur,
// cmd/spur-server) already carries a stable "app: <stage>: ..." /
// "server: <stage>: ..." prefix identifying which step failed (see e.g.
// cmd/spur/tunnel.go's rendezvous — "stun discovery", "exchange
// candidates", "dial control-plane", ...). Explain matches on those
// stage markers, plus the underlying error's real cause where that
// matters (io.EOF, context.DeadlineExceeded, os.IsNotExist, ...), to
// give stage-specific advice instead of a bare Go error string. This is
// deliberately string-matching against our own controlled prefixes (not
// user input, and not third-party error text) rather than threading a
// typed sentinel error through every call site in cmd/*  — pragmatic for
// a UX layer that always has a safe fallback (the original message) when
// it doesn't recognize something, so a wording change elsewhere degrades
// gracefully instead of silently breaking.
func Explain(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()

	switch {
	case strings.Contains(msg, "stun discovery"):
		return friendly(msg,
			"Не удалось связаться со STUN-сервером (--stun-server).",
			"Проверьте: адрес и порт указаны верно; порт открыт по UDP и на этой машине, и на сервере (файрвол/security group у облачного провайдера часто блокирует UDP по умолчанию, даже если TCP открыт).")

	case strings.Contains(msg, "exchange candidates"):
		return friendly(msg,
			"Второй участник не ответил вовремя (или указан не тот собеседник).",
			"Убедитесь, что второй участник запустил соответствующую команду (например `spur receive`) в течение примерно минуты после вас и указал в --to именно ваш ТЕКУЩИЙ peer-id — сверьте свежим `spur whoami`, не значением из старой истории команд.")

	case strings.Contains(msg, "dial control-plane"):
		return friendly(msg,
			"Не удалось подключиться к серверу (--server).",
			"Проверьте: spur-server запущен и слушает; адрес/порт указаны верно; порт открыт по UDP снаружи.")

	case strings.Contains(msg, "establish session"):
		return friendly(msg,
			"Не удалось установить канал — ни напрямую (P2P), ни через relay-сервер.",
			"Оба способа не сработали одновременно — обычно значит, что у одной из сторон нестабильное соединение или порт сервера недоступен по UDP. Проверьте связь: ping/mtr до сервера, и что оба порта сервера (--listen и --stun-listen) открыты снаружи.")

	case strings.Contains(msg, "wait for receiver ack") || strings.Contains(msg, "stream closed") || strings.Contains(msg, "keepalive"):
		return friendly(msg,
			"Соединение оборвалось во время передачи данных.",
			"Файл мог дойти не полностью — запустите передачу заново.")

	case strings.Contains(msg, "invalid invite token") || strings.Contains(msg, "invalid or missing invite token"):
		return friendly(msg,
			"Неверный или отсутствующий инвайт-токен.",
			"Токен нужен только для входа в УЖЕ существующую сеть — уточните его у того, кто её создавал (он печатается при первом `spur join`/`spur join-network` для этой сети).")

	case strings.Contains(msg, "address already in use"):
		return friendly(msg,
			"Порт уже занят другим процессом.",
			"Проверьте `ss -ulpn | grep <порт>` — возможно, где-то уже работает второй spur-server/spur, или порт совпадает с другим сервисом. Частая ошибка: --listen и --stun-listen указаны одинаковыми — это два разных порта.")

	case strings.Contains(msg, "connection refused"):
		return friendly(msg,
			"Сервер отверг подключение.",
			"Проверьте, что spur-server запущен на этом адресе и порту.")

	case errors.Is(err, fs.ErrNotExist):
		return friendly(msg,
			"Файл или директория не найдены.",
			"Проверьте путь — частая причина: относительный путь (`./mnt/...`) интерпретируется от текущей директории, а не от корня диска (`/mnt/...`).")

	case errors.Is(err, fs.ErrPermission):
		return friendly(msg,
			"Недостаточно прав доступа.",
			"")

	case errors.Is(err, context.DeadlineExceeded):
		return friendly(msg,
			"Истекло время ожидания.",
			"")

	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		return friendly(msg,
			"Соединение закрылось раньше, чем ожидалось.",
			"")
	}

	return msg
}

// friendly composes the headline (always shown), an optional hint
// (omitted when empty), and the original technical message (always
// shown, so nothing is ever hidden behind the friendly wording).
func friendly(original, headline, hint string) string {
	s := headline
	if hint != "" {
		s += "\n  " + hint
	}
	s += "\n  Технические детали: " + original
	return s
}
