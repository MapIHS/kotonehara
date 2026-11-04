package config

import (
	"os"
	"testing"
)

func TestOpenAISystemPrompt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		unset bool
		want  string
	}{
		{name: "unset", unset: true},
		{name: "empty"},
		{name: "whitespace", value: " \n\t "},
		{name: "explicit", value: "Gunakan instruksi khusus pemilik.", want: "Gunakan instruksi khusus pemilik."},
		{name: "trim outer whitespace", value: " \nInstruksi khusus.\nBaris kedua.\t ", want: "Instruksi khusus.\nBaris kedua."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OPENAI_SYSTEM_PROMPT", tc.value)
			if tc.unset {
				if err := os.Unsetenv("OPENAI_SYSTEM_PROMPT"); err != nil {
					t.Fatal(err)
				}
			}
			if got := Load().OpenAISystemPrompt; got != tc.want {
				t.Fatalf("OpenAISystemPrompt = %q, want %q", got, tc.want)
			}
		})
	}
}
