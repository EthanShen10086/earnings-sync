package acquire

import (
	"regexp"
	"strings"
)

var (
	rowRE  = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	cellRE = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	tagRE  = regexp.MustCompile(`(?is)<[^>]+>`)
)

// StatementText keeps filing headlines and table rows, and drops the HTML chrome.
// A raw exhibit is too large to send to a model unchanged.
func StatementText(body string) string {
	if !strings.Contains(strings.ToLower(body), "<tr") {
		return body
	}
	var b strings.Builder
	plain := collapse(tagRE.ReplaceAllString(strings.ReplaceAll(body, "<br>", "\n"), " "))
	if len(plain) > 1200 {
		plain = plain[:1200]
	}
	b.WriteString(plain)
	b.WriteByte('\n')
	for _, row := range rowRE.FindAllStringSubmatch(body, -1) {
		cells := cellRE.FindAllStringSubmatch(row[1], -1)
		var parts []string
		for _, cell := range cells {
			text := collapse(tagRE.ReplaceAllString(cell[1], " "))
			if text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			continue
		}
		b.WriteString(strings.Join(parts, " | "))
		b.WriteByte('\n')
	}
	out := b.String()
	if len(out) > 24000 {
		return out[:24000]
	}
	return out
}

func collapse(s string) string {
	replacer := strings.NewReplacer(
		"&nbsp;", " ",
		"&#160;", " ",
		"&amp;", "&",
		"&#8217;", "'",
		"&#8212;", "-",
		"&#58;", ":",
	)
	s = replacer.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
