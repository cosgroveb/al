package anylist

import (
	"sort"
	"strings"
	"unicode"
)

// categoryLower applies Unicode 17 default lowercasing before the legacy filter.
// Sigma context uses original runes, including astral characters, without a bound.
func categoryLower(name string) string {
	var result strings.Builder
	result.Grow(len(name))
	precededByCased := false
	for i, r := range name {
		switch {
		case r == '\u0130':
			result.WriteString("i\u0307")
		case r == 'Σ' && precededByCased && !categoryFollowedByCased(name[i+len("Σ"):]):
			result.WriteRune('ς')
		default:
			result.WriteRune(categorySimpleLower(r))
		}
		if !unicode.Is(categoryCaseIgnorable, r) {
			precededByCased = unicode.Is(categoryCased, r)
		}
	}
	return result.String()
}

func categoryFollowedByCased(name string) bool {
	for _, r := range name {
		if !unicode.Is(categoryCaseIgnorable, r) {
			return unicode.Is(categoryCased, r)
		}
	}
	return false
}

func categorySimpleLower(r rune) rune {
	i := sort.Search(len(categoryLowerRanges), func(i int) bool { return categoryLowerRanges[i].hi >= r })
	if i < len(categoryLowerRanges) {
		mapping := categoryLowerRanges[i]
		if r >= mapping.lo && (r-mapping.lo)%mapping.stride == 0 {
			return r + mapping.delta
		}
	}
	return r
}
