package anylist

import "testing"

func TestCategoryLowerExpansion(t *testing.T) {
	for _, test := range []struct {
		name, lower string
	}{
		{"İ", "i\u0307"},
		{"İΣ", "i\u0307ς"},
		{"Σİ", "σi\u0307"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := categoryLower(test.name); got != test.lower {
				t.Fatalf("categoryLower(%q) = %q, want %q", test.name, got, test.lower)
			}
		})
	}
}
