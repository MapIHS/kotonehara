package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AFKState struct {
	IsAFK   bool      `json:"is_afk"`
	Reason  string    `json:"reason"`
	Time    time.Time `json:"time"`
}

var (
	afkStore = make(map[string]AFKState)
	afkMu    sync.RWMutex
	storeDir = "data"
	afkFile  = "afk_store.json"
)

func init() {
	LoadAFKStore()
}

func GetAFKPath() string {
	return filepath.Join(storeDir, afkFile)
}

func LoadAFKStore() {
	afkMu.Lock()
	defer afkMu.Unlock()
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return
	}
	b, err := os.ReadFile(GetAFKPath())
	if err == nil {
		_ = json.Unmarshal(b, &afkStore)
	}
}

func SaveAFKStore() {
	b, err := json.Marshal(afkStore)
	if err == nil {
		_ = os.WriteFile(GetAFKPath(), b, 0644)
	}
}

func SetAFK(jid string, reason string) {
	afkMu.Lock()
	defer afkMu.Unlock()
	afkStore[jid] = AFKState{
		IsAFK:  true,
		Reason: reason,
		Time:   time.Now(),
	}
	SaveAFKStore()
}

func ClearAFK(jid string) (AFKState, bool) {
	afkMu.Lock()
	defer afkMu.Unlock()
	state, exists := afkStore[jid]
	if exists && state.IsAFK {
		delete(afkStore, jid)
		SaveAFKStore()
		return state, true
	}
	return AFKState{}, false
}

func GetAFK(jid string) (AFKState, bool) {
	afkMu.RLock()
	defer afkMu.RUnlock()
	state, exists := afkStore[jid]
	return state, exists && state.IsAFK
}
