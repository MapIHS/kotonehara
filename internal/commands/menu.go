package commands

import (
	"sort"
	"strings"
)

func BuildMenuText(prefix string) string {
	tags := groupByTag()

	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		b.WriteString(" *")
		b.WriteString(strings.ToUpper(key))
		b.WriteString("*\n")

		cmds := append([]*Command(nil), tags[key]...)
		sort.Slice(cmds, func(i, j int) bool {
			return strings.ToLower(cmds[i].Name) < strings.ToLower(cmds[j].Name)
		})

		for _, c := range cmds {
			p := ""
			if c.IsPrefix {
				p = prefix
			}
			name := c.Name
			if name == "" && len(c.As) > 0 {
				name = c.As[0]
			}

			b.WriteString(" - ")
			b.WriteString(p)
			b.WriteString(name)

			if len(c.As) > 0 {
				var al []string
				for _, a := range c.As {
					if strings.EqualFold(a, name) {
						continue
					}
					al = append(al, p+a)
				}
				if len(al) > 0 {
					b.WriteString(" (alias: ")
					b.WriteString(strings.Join(al, ", "))
					b.WriteString(")")
				}
			}

			if d := strings.TrimSpace(c.Description); d != "" {
				b.WriteString(" — ")
				b.WriteString(d)
			}
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	return b.String()
}
