package commands

import (
	"strings"
	"sync"

	"github.com/MapIHS/kotonehara/internal/infra/config"
)

type registry struct {
	mu    sync.RWMutex
	list  []Command
	byKey map[string]*Command
	byTag map[string][]*Command
}

var reg = &registry{
	byKey: map[string]*Command{},
	byTag: map[string][]*Command{},
}

func Register(cmd *Command) {
	if cmd == nil || strings.TrimSpace(cmd.Name) == "" {
		return
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()

	reg.list = append(reg.list, *cmd)
	ptr := &reg.list[len(reg.list)-1]

	// key utama
	k := strings.ToLower(strings.TrimSpace(cmd.Name))
	reg.byKey[k] = ptr

	// alias
	for _, a := range cmd.As {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		reg.byKey[a] = ptr
	}

	// tag
	tag := strings.TrimSpace(cmd.Tags)
	reg.byTag[tag] = append(reg.byTag[tag], ptr)
}

func Commands() []Command {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	out := make([]Command, len(reg.list))
	copy(out, reg.list)
	return out
}

func lookup(key string) (*Command, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return nil, false
	}

	reg.mu.RLock()
	defer reg.mu.RUnlock()

	c, ok := reg.byKey[key]
	return c, ok
}

func CanHandle(body string, cfg config.Config) bool {
	s := strings.TrimSpace(body)
	if s == "" {
		return false
	}

	cfgPrefix := strings.TrimSpace(cfg.Prefix)
	msgPrefix := ""
	if cfgPrefix != "" && strings.HasPrefix(s, cfgPrefix) {
		msgPrefix = cfgPrefix
	}

	name := s
	if msgPrefix != "" {
		name = strings.TrimSpace(strings.TrimPrefix(name, msgPrefix))
	}

	parts := strings.Fields(name)
	if len(parts) == 0 {
		return false
	}

	cmd, ok := lookup(parts[0])
	if !ok {
		return false
	}
	if cmd.IsPrefix && msgPrefix == "" {
		return false
	}
	if !cmd.IsPrefix && msgPrefix != "" {
		return false
	}
	return true
}

func groupByTag() map[string][]*Command {
	reg.mu.RLock()
	defer reg.mu.RUnlock()

	out := make(map[string][]*Command, len(reg.byTag))
	for k, v := range reg.byTag {
		tmp := make([]*Command, len(v))
		copy(tmp, v)
		out[k] = tmp
	}
	return out
}
