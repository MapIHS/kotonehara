package handlers

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestTrustedSourceVariants(t *testing.T) {
	ws, _ := url.Parse("wss://api.oohara.dev/ws/player/abc")
	want := map[string][]string{
		"A host (sekarang)":    {"api.oohara.dev"},
		"B origin https":       {"https://api.oohara.dev"},
		"C origin wss":         {"wss://api.oohara.dev"},
		"D https + wss + host": {"https://api.oohara.dev", "wss://api.oohara.dev", "api.oohara.dev"},
		"E domain induk":       {"oohara.dev"},
		"F kosong":             nil,
	}
	variants := trustedSourceVariants()
	if len(variants) != len(want) {
		t.Fatalf("got %d variants", len(variants))
	}
	for _, v := range variants {
		if got := v.Sources(ws); !reflect.DeepEqual(got, want[v.Label]) {
			t.Errorf("%s: got %q want %q", v.Label, got, want[v.Label])
		}
	}
	// The session path must never leak into a trusted source.
	for _, v := range variants {
		for _, s := range v.Sources(ws) {
			if len(s) > 0 && (strings.Contains(s, "/ws/") || strings.Contains(s, "abc")) {
				t.Errorf("%s leaked the session path: %q", v.Label, s)
			}
		}
	}
}
