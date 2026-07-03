package cli

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCatalog_NoEmptyFields guards against a field added to the catalog
// struct but forgotten in one of the two language instances — with this
// many fields (see catalog.go's doc comment), that's an easy typo to
// make and gofmt/go vet can't catch a merely-empty string the way they'd
// catch a missing field entirely. A zero-value string field silently
// prints nothing instead of failing to compile.
func TestCatalog_NoEmptyFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		cat  catalog
	}{
		{"ru", ruCatalog},
		{"en", enCatalog},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := reflect.ValueOf(tc.cat)
			typ := v.Type()
			for i := 0; i < v.NumField(); i++ {
				require.NotEmpty(t, v.Field(i).String(), "catalog.%s is empty in %q", typ.Field(i).Name, tc.name)
			}
		})
	}
}
