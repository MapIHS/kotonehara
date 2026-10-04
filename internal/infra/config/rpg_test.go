package config

import "testing"

func TestRPGListenAddress(t *testing.T) {
	for _, tc := range []struct{ name, port, explicit, want string }{
		{"default local", "", "", "127.0.0.1:8089"},
		{"VPS", "", "0.0.0.0:8089", "0.0.0.0:8089"},
		{"Heroku dynamic port", " 19384 ", "", "0.0.0.0:19384"},
		{"Heroku overrides stale VPS setting", "19384", "0.0.0.0:8089", "0.0.0.0:19384"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PORT", tc.port)
			t.Setenv("RPG_LISTEN_ADDR", tc.explicit)
			if got := Load().RPGListenAddr; got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
