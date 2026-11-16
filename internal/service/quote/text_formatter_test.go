package quote

import (
	"reflect"
	"testing"
)

func TestParseEntities(t *testing.T) {
	for _, tc := range []struct {
		in   string
		text string
		want []Entity
	}{
		{"plain", "plain", nil},
		{"a *bold* b", "a bold b", []Entity{{Type: "bold", Offset: 2, Length: 4}}},
		{"_x_ ~y~", "x y", []Entity{{Type: "italic", Offset: 0, Length: 1}, {Type: "strikethrough", Offset: 2, Length: 1}}},
		{"```code``` *b*", "code b", []Entity{{Type: "pre", Offset: 0, Length: 4}, {Type: "bold", Offset: 5, Length: 1}}},
		// Offsets are UTF-16 units: the emoji counts as 2.
		{"😀 *hi*", "😀 hi", []Entity{{Type: "bold", Offset: 3, Length: 2}}},
	} {
		text, got := ParseEntities(tc.in)
		if text != tc.text {
			t.Errorf("%q: text %q, want %q", tc.in, text, tc.text)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: entities %+v, want %+v", tc.in, got, tc.want)
		}
	}
}
