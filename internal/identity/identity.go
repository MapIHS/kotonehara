package identity

import (
	"fmt"
	"strings"
	"unicode"

	"go.mau.fi/whatsmeow/types"
)

type Identity struct {
	Primary types.JID
	PN      types.JID
	LID     types.JID
	Key     string
}

func New(primary, alternate types.JID) Identity {
	id := Identity{Primary: Normalize(primary)}
	id.add(primary)
	id.add(alternate)
	id.refreshKey()
	return id
}

func Normalize(jid types.JID) types.JID {
	if jid.IsEmpty() {
		return types.EmptyJID
	}
	return jid.ToNonAD()
}

func ParseUser(value string) (types.JID, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return types.EmptyJID, fmt.Errorf("JID kosong")
	}
	if strings.Contains(value, "@") {
		jid, err := types.ParseJID(value)
		if err != nil || jid.IsEmpty() {
			return types.EmptyJID, fmt.Errorf("JID tidak valid")
		}
		jid = Normalize(jid)
		if jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer {
			return types.EmptyJID, fmt.Errorf("namespace JID tidak didukung")
		}
		return jid, nil
	}
	value = strings.TrimPrefix(value, "+")
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return !unicode.IsDigit(r) }) >= 0 {
		return types.EmptyJID, fmt.Errorf("nomor tidak valid")
	}
	return types.NewJID(value, types.DefaultUserServer), nil
}

func (id *Identity) Add(jid types.JID) {
	id.add(jid)
	id.refreshKey()
}

func (id *Identity) add(jid types.JID) {
	jid = Normalize(jid)
	if jid.IsEmpty() {
		return
	}
	switch jid.Server {
	case types.DefaultUserServer:
		id.PN = jid
	case types.HiddenUserServer:
		id.LID = jid
	}
}

func (id *Identity) refreshKey() {
	switch {
	case !id.PN.IsEmpty():
		id.Key = "pn:" + id.PN.User
	case !id.LID.IsEmpty():
		id.Key = "lid:" + id.LID.User
	case !id.Primary.IsEmpty():
		id.Key = id.Primary.Server + ":" + id.Primary.User
	default:
		id.Key = ""
	}
}

func (id Identity) Aliases() []types.JID {
	aliases := make([]types.JID, 0, 3)
	seen := make(map[string]struct{}, 3)
	for _, jid := range []types.JID{id.Primary, id.PN, id.LID} {
		jid = Normalize(jid)
		if jid.IsEmpty() {
			continue
		}
		key := jid.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		aliases = append(aliases, jid)
	}
	return aliases
}

func (id Identity) AliasStrings() []string {
	aliases := id.Aliases()
	result := make([]string, 0, len(aliases))
	for _, jid := range aliases {
		result = append(result, jid.String())
	}
	return result
}

func (id Identity) Matches(jid types.JID) bool {
	jid = Normalize(jid)
	if jid.IsEmpty() {
		return false
	}
	for _, alias := range id.Aliases() {
		if alias == jid {
			return true
		}
	}
	return false
}

func (id Identity) MatchesString(raw string) bool {
	jid, err := ParseUser(raw)
	return err == nil && id.Matches(jid)
}

func (id Identity) StateJID() string {
	if !id.PN.IsEmpty() {
		return id.PN.String()
	}
	if !id.LID.IsEmpty() {
		return id.LID.String()
	}
	return Normalize(id.Primary).String()
}
