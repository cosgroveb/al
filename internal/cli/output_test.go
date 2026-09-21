package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cosgroveb/al/internal/anylist"
)

func TestHumanOutputEscapesControlsAndShowsItemFields(t *testing.T) {
	item := anylist.Item{
		ID: "item-1", Name: "milk\x1b[2J\n", Checked: true, Quantity: "2\tboxes",
		Notes: "first\nsecond\r", CategoryID: "category-1", CategoryName: "Dairy\u202e",
	}
	text := humanData(map[string]any{"items": []anylist.Item{item}}, false)
	for _, want := range []string{`[x] milk\x1b[2J\n (item-1)`, `Quantity: 2\tboxes`, `Notes: first\nsecond\r`, `Category: Dairy\u202e (category-1)`} {
		if !strings.Contains(text, want) {
			t.Fatalf("output %q lacks %q", text, want)
		}
	}
	if strings.ContainsAny(text, "\x1b\t\r\u202e") {
		t.Fatalf("raw terminal controls in %q", text)
	}
	var out bytes.Buffer
	a := newApp(nil, &out, nil, options{})
	a.json = true
	a.data = map[string]any{"items": []anylist.Item{item}}
	if err := a.render(nil); err != nil {
		t.Fatal(err)
	}
	got := decodeShellOutput(t, &out)
	var data struct {
		Items []anylist.Item `json:"items"`
	}
	if err := json.Unmarshal(got.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 1 || data.Items[0].Name != item.Name || data.Items[0].Notes != item.Notes {
		t.Fatalf("JSON changed item data: %+v", data.Items)
	}
}

func TestColorIsExplicitAndDisabledForPipes(t *testing.T) {
	if strings.Contains(styled("name", false), "\x1b") || !strings.Contains(styled("name", true), "\x1b") {
		t.Fatal("style selection did not control ANSI output")
	}
	for _, environment := range []map[string]string{{}, {"TERM": "dumb"}, {"NO_COLOR": "1"}} {
		a := newApp(nil, nil, nil, options{getenv: func(name string) string { return environment[name] }})
		if a.useColor(new(bytes.Buffer)) {
			t.Fatal("non-terminal output used color")
		}
	}
}

func TestEmptyCollectionsAndPartialResults(t *testing.T) {
	index := 1
	for _, data := range []map[string]any{
		{"lists": []anylist.List{}},
		{"items": []anylist.Item{}},
		{"categories": []anylist.Category{}},
		{"results": []anylist.MutationResult{}},
		{"results": []anylist.MutationResult{{Index: &index, ID: "item-1", Outcome: "unknown"}}},
	} {
		var out bytes.Buffer
		a := newApp(nil, &out, nil, options{})
		a.json, a.data = true, data
		problem := &anylist.Error{Code: "transport", Message: "request did not complete"}
		if err := a.render(problem); err != nil {
			t.Fatal(err)
		}
		got := decodeShellOutput(t, &out)
		if got.OK || got.Error == nil || got.Error.Code != "transport" {
			t.Fatalf("result = %+v", got)
		}
		if bytes.Contains(got.Data, []byte("null")) {
			t.Fatalf("collection changed to null: %s", got.Data)
		}
		if text := humanData(data, false); text == "" {
			t.Fatalf("empty human result for %+v", data)
		}
	}
}
