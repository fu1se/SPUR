package infra_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fu1se/spur/internal/infra"
)

func TestDetectSystemLanguage(t *testing.T) {
	cases := []struct {
		name               string
		lcAll, lcMsg, lang string
		want               string
	}{
		{"all unset defaults to en", "", "", "", "en"},
		{"LANG=ru_RU.UTF-8", "", "", "ru_RU.UTF-8", "ru"},
		{"LANG=en_US.UTF-8", "", "", "en_US.UTF-8", "en"},
		{"LANG=de_DE.UTF-8 (unsupported, defaults to en)", "", "", "de_DE.UTF-8", "en"},
		{"LC_ALL overrides LANG", "en_US.UTF-8", "", "ru_RU.UTF-8", "en"},
		{"LC_MESSAGES overrides LANG", "", "ru_RU.UTF-8", "en_US.UTF-8", "ru"},
		{"LC_ALL overrides LC_MESSAGES", "ru_RU.UTF-8", "en_US.UTF-8", "", "ru"},
		{"bare 'ru' locale", "", "", "ru", "ru"},
		{"bare 'C' locale defaults to en", "", "", "C", "en"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("LC_ALL", c.lcAll)
			t.Setenv("LC_MESSAGES", c.lcMsg)
			t.Setenv("LANG", c.lang)
			require.Equal(t, c.want, infra.DetectSystemLanguage())
		})
	}
}
