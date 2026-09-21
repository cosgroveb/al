package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
	"github.com/cosgroveb/al/internal/anylist"
)

type envelope struct {
	OK    bool           `json:"ok"`
	Data  any            `json:"data"`
	Error *anylist.Error `json:"error"`
}

func (a *app) render(diagnostic *anylist.Error) error {
	if a.json {
		return json.NewEncoder(a.out).Encode(envelope{OK: diagnostic == nil, Data: a.data, Error: diagnostic})
	}
	if a.data != nil {
		if _, err := io.WriteString(a.out, humanData(a.data, a.useColor(a.out))); err != nil {
			return err
		}
	}
	if diagnostic != nil {
		var message strings.Builder
		fmt.Fprintf(&message, "%s: %s\n", styled("error", a.useColor(a.errOut)), safeText(diagnostic.Message))
		for _, candidate := range diagnostic.Candidates {
			fmt.Fprintf(&message, "  %s (%s)\n", safeText(candidate.Name), safeText(candidate.ID))
		}
		_, err := io.WriteString(a.errOut, message.String())
		return err
	}
	return nil
}

func (a *app) useColor(out io.Writer) bool {
	if a.opts.getenv("TERM") == "dumb" || a.opts.getenv("NO_COLOR") != "" {
		return false
	}
	file, ok := out.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(file.Fd())
}

func styled(text string, color bool) string {
	if !color {
		return text
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7DC4E4")).Render(text)
}

func safeText(text string) string {
	var safe strings.Builder
	for _, r := range text {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			quoted := strconv.QuoteRune(r)
			safe.WriteString(quoted[1 : len(quoted)-1])
		} else {
			safe.WriteRune(r)
		}
	}
	return safe.String()
}

func humanData(data any, color bool) string {
	var out strings.Builder
	fields, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	if text, ok := fields["text"].(string); ok {
		return text
	}
	if authenticated, ok := fields["authenticated"].(bool); ok && authenticated {
		fmt.Fprintf(&out, "%s\n", styled("Authenticated with AnyList.", color))
	}
	if list, ok := fields["list"].(anylist.List); ok {
		fmt.Fprintf(&out, "%s (%s)\n", styled(safeText(list.Name), color), safeText(list.ID))
	}
	if lists, ok := fields["lists"].([]anylist.List); ok {
		if len(lists) == 0 {
			out.WriteString("No lists.\n")
		}
		for _, list := range lists {
			fmt.Fprintf(&out, "%s (%s)\n", styled(safeText(list.Name), color), safeText(list.ID))
		}
	}
	if items, ok := fields["items"].([]anylist.Item); ok {
		if len(items) == 0 {
			out.WriteString("No items.\n")
		}
		for _, item := range items {
			writeItem(&out, item, color)
		}
	}
	if categories, ok := fields["categories"].([]anylist.Category); ok {
		if len(categories) == 0 {
			out.WriteString("No categories.\n")
		}
		for _, category := range categories {
			fmt.Fprintf(&out, "%s (%s)\n", styled(safeText(category.Name), color), safeText(category.ID))
		}
	}
	if results, ok := fields["results"].([]anylist.MutationResult); ok {
		if len(results) == 0 {
			out.WriteString("No changes.\n")
		}
		for _, result := range results {
			if result.Index != nil {
				fmt.Fprintf(&out, "[%d] ", *result.Index)
			}
			fmt.Fprintf(&out, "%s", styled(safeText(result.Outcome), color))
			if result.List != nil {
				fmt.Fprintf(&out, " %s", safeText(result.List.Name))
			}
			fmt.Fprintf(&out, " (%s)\n", safeText(result.ID))
			if result.Item != nil {
				writeItem(&out, *result.Item, color)
			}
		}
	}
	if config, ok := fields["config"].(Config); ok {
		if path, ok := fields["path"].(string); ok {
			fmt.Fprintf(&out, "Config: %s\n", safeText(path))
		}
		fmt.Fprintf(&out, "Client ID: %s\nDefault list ID: %s\n", safeText(config.ClientID), safeText(config.DefaultListID))
	}
	return out.String()
}

func writeItem(out *strings.Builder, item anylist.Item, color bool) {
	checked := " "
	if item.Checked {
		checked = "x"
	}
	fmt.Fprintf(out, "[%s] %s (%s)\n", checked, styled(safeText(item.Name), color), safeText(item.ID))
	if item.Quantity != "" {
		fmt.Fprintf(out, "    Quantity: %s\n", safeText(item.Quantity))
	}
	if item.Notes != "" {
		fmt.Fprintf(out, "    Notes: %s\n", safeText(item.Notes))
	}
	if item.CategoryID != "" || item.CategoryName != "" {
		fmt.Fprintf(out, "    Category: %s (%s)\n", safeText(item.CategoryName), safeText(item.CategoryID))
	}
}
