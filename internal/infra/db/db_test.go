package db

import (
	"context"
	"testing"

	_ "modernc.org/sqlite"
)

func TestConnectRequiresURL(t *testing.T) {
	if _, err := Connect(context.Background(), Config{Driver: "sqlite"}); err == nil {
		t.Fatal("empty URL accepted")
	}
}

func TestConnectSQLiteAppliesPool(t *testing.T) {
	conn, err := Connect(context.Background(), Config{Driver: "sqlite", URL: ":memory:", MaxOpenConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := conn.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConns = %d", got)
	}
}

func TestConnectUnknownDriverFails(t *testing.T) {
	if _, err := Connect(context.Background(), Config{Driver: "nope", URL: "x"}); err == nil {
		t.Fatal("unknown driver accepted")
	}
}
