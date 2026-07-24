package quote

import (
	"regexp"
	"unicode/utf16"
)

// We define simple regexes for WhatsApp formatting.
// WhatsApp formatting rules are roughly: must not be preceded by alphanumeric,
// and inside the markers must not be completely empty.
// To keep it simple but functional, we match the markers and their content.
var (
	codeRe   = regexp.MustCompile("```([^`]+?)```")
	boldRe   = regexp.MustCompile(`\*([^*\n]+?)\*`)
	italicRe = regexp.MustCompile(`_([^_\n]+?)_`)
	strikeRe = regexp.MustCompile(`~([^~\n]+?)~`)
)

// ParseEntities converts WhatsApp markdown into Telegram entities.
// It removes the markdown markers and returns the clean text with its entities.
// The offsets and lengths are computed in UTF-16 code units as required by Telegram API.
func ParseEntities(text string) (string, []Entity) {
	var entities []Entity

	text = extractFormat(text, codeRe, "pre", &entities)
	text = extractFormat(text, boldRe, "bold", &entities)
	text = extractFormat(text, italicRe, "italic", &entities)
	text = extractFormat(text, strikeRe, "strikethrough", &entities)

	return text, entities
}

// extractFormat finds all matches of a regex, removes the formatting characters,
// and adds the new entities. It shifts previous entities if they appear after the match.
func extractFormat(text string, re *regexp.Regexp, entityType string, entities *[]Entity) string {
	for {
		loc := re.FindStringSubmatchIndex(text)
		if loc == nil {
			break
		}

		matchStart := loc[0]
		matchEnd := loc[1]
		groupStart := loc[2]
		groupEnd := loc[3]

		content := text[groupStart:groupEnd]

		utf16Before := len(utf16.Encode([]rune(text[:matchStart])))
		utf16Content := len(utf16.Encode([]rune(content)))

		newEntity := Entity{
			Type:   entityType,
			Offset: utf16Before,
			Length: utf16Content,
		}
		*entities = append(*entities, newEntity)

		utf16PrefixRemoved := len(utf16.Encode([]rune(text[matchStart:groupStart])))
		utf16SuffixRemoved := len(utf16.Encode([]rune(text[groupEnd:matchEnd])))

		newEntityIndex := len(*entities) - 1
		for i := range *entities {
			if i == newEntityIndex {
				continue
			}
			e := &(*entities)[i]

			if e.Offset >= utf16Before+utf16PrefixRemoved {
				e.Offset -= utf16PrefixRemoved
				if e.Offset >= utf16Before+utf16Content+utf16SuffixRemoved {
					e.Offset -= utf16SuffixRemoved
				}
			}

			if e.Offset <= utf16Before && e.Offset+e.Length >= utf16Before+utf16PrefixRemoved {
				e.Length -= utf16PrefixRemoved
			}
			if e.Offset <= utf16Before+utf16Content && e.Offset+e.Length >= utf16Before+utf16Content+utf16SuffixRemoved {
				e.Length -= utf16SuffixRemoved
			}
		}

		text = text[:matchStart] + content + text[matchEnd:]
	}

	return text
}
