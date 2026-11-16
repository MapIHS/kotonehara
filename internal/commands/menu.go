package commands

import (
	"sort"
	"strings"
)

// HelpFlag, given as a command's only argument, shows that command's
// description instead of running it (e.g. ".s --d").
const HelpFlag = "--d"

// IsHelpFlag reports whether a command query asks for the description.
func IsHelpFlag(query string) bool {
	return strings.EqualFold(strings.TrimSpace(query), HelpFlag)
}

// visibleIn mirrors the chat restrictions enforced by CommandExec so the menu
// and --d never advertise a command that cannot be used in this chat.
func visibleIn(c *Command, isGroup, nsfwEnabled bool) bool {
	if !isGroup || !c.IsPrivate {
		return true
	}
	return nsfwEnabled && strings.Contains(strings.ToLower(c.Tags), "nsfw")
}

func displayName(c *Command) string {
	if c.Name == "" && len(c.As) > 0 {
		return c.As[0]
	}
	return c.Name
}

func commandPrefix(c *Command, prefix string) string {
	if c.IsPrefix {
		return prefix
	}
	return ""
}

func aliasList(c *Command, prefix string) []string {
	name := displayName(c)
	p := commandPrefix(c, prefix)
	var al []string
	for _, a := range c.As {
		if strings.EqualFold(a, name) {
			continue
		}
		al = append(al, p+a)
	}
	return al
}

// BuildMenuText lists commands grouped by tag. Descriptions are included only
// when withDescription is true (".menu --d").
func BuildMenuText(prefix string, isGroup, nsfwEnabled, withDescription bool) string {
	tags := groupByTag()

	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		var cmds []*Command
		for _, c := range tags[key] {
			if visibleIn(c, isGroup, nsfwEnabled) {
				cmds = append(cmds, c)
			}
		}
		if len(cmds) == 0 {
			continue
		}

		sort.Slice(cmds, func(i, j int) bool {
			return strings.ToLower(cmds[i].Name) < strings.ToLower(cmds[j].Name)
		})

		b.WriteString(" *")
		b.WriteString(strings.ToUpper(key))
		b.WriteString("*\n")

		for _, c := range cmds {
			b.WriteString(" - ")
			b.WriteString(commandPrefix(c, prefix))
			b.WriteString(displayName(c))

			if al := aliasList(c, prefix); len(al) > 0 {
				b.WriteString(" (alias: ")
				b.WriteString(strings.Join(al, ", "))
				b.WriteString(")")
			}

			if d := strings.TrimSpace(c.Description); withDescription && d != "" {
				b.WriteString("\n   ")
				b.WriteString(d)
			}
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}

// BuildCommandHelp describes one command for "<command> --d".
func BuildCommandHelp(c *Command, prefix string) string {
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(commandPrefix(c, prefix))
	b.WriteString(displayName(c))
	b.WriteString("*\n")

	if d := strings.TrimSpace(c.Description); d != "" {
		b.WriteString(d)
	} else {
		b.WriteString("Belum ada deskripsi.")
	}
	b.WriteString("\n")

	if al := aliasList(c, prefix); len(al) > 0 {
		b.WriteString("\nAlias: ")
		b.WriteString(strings.Join(al, ", "))
	}
	if tag := strings.TrimSpace(c.Tags); tag != "" {
		b.WriteString("\nKategori: ")
		b.WriteString(tag)
	}

	var req []string
	if c.IsQuery {
		req = append(req, "butuh teks/link")
	}
	if c.IsMedia {
		req = append(req, "butuh media (kirim atau balas)")
	}
	if c.IsGroup {
		req = append(req, "hanya di grup")
	}
	if c.IsPrivate {
		req = append(req, "hanya di chat pribadi")
	}
	if c.IsAdmin {
		req = append(req, "khusus admin grup")
	}
	if c.IsBotAdmin {
		req = append(req, "bot harus admin")
	}
	if c.IsOwner {
		req = append(req, "khusus owner")
	}
	if len(req) > 0 {
		b.WriteString("\nSyarat: ")
		b.WriteString(strings.Join(req, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}
