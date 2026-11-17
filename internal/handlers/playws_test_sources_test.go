package handlers

import (
	"net/url"
	"strings"
	"testing"
)

func TestCardVariants(t *testing.T) {
	ws, _ := url.Parse("wss://api.oohara.dev/ws/player/abc")
	const fragment = `<section class="kt-player">x</section>`
	variants := cardVariants()
	if len(variants) != 4 {
		t.Fatalf("got %d variants", len(variants))
	}
	if got := variants[0].HTML(fragment, "T", ws); got != fragment {
		t.Fatalf("fragment variant changed the payload: %q", got)
	}
	for _, v := range variants[1:] {
		doc := v.HTML(fragment, `Judul "<b>"`, ws)
		if !strings.HasPrefix(doc, "<!DOCTYPE html><html") || !strings.HasSuffix(doc, "</body></html>") || !strings.Contains(doc, fragment) {
			t.Errorf("%s: not a complete document: %q", v.Label, doc)
		}
		if strings.Contains(doc, "<b>") {
			t.Errorf("%s: title not escaped", v.Label)
		}
	}
	csp := variants[2].HTML(fragment, "T", ws)
	if !strings.Contains(csp, "connect-src wss://api.oohara.dev https://api.oohara.dev") {
		t.Errorf("meta CSP missing connect-src: %q", csp)
	}
	for _, v := range variants {
		for _, s := range v.Sources(ws) {
			if strings.Contains(s, "/ws/") || strings.Contains(s, "abc") {
				t.Errorf("%s leaked the session path: %q", v.Label, s)
			}
		}
		if strings.Contains(v.HTML(fragment, "T", ws), "abc") {
			t.Errorf("%s leaked the session id into the document", v.Label)
		}
	}
}
