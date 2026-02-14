package filter

import (
	"strings"

	"github.com/amishk599/firstin/internal/model"
)

// TitleAndLocationFilter matches jobs whose title contains any of the title
// keywords and whose location contains any of the location keywords.
// Matching is case-insensitive. Empty keyword lists are treated as "match all".
type TitleAndLocationFilter struct {
	titleKeywords []string
	locations     []string
}

// NewTitleAndLocationFilter returns a filter that requires both a title keyword
// match and a location keyword match (case-insensitive substring).
func NewTitleAndLocationFilter(titleKeywords []string, locations []string) *TitleAndLocationFilter {
	return &TitleAndLocationFilter{
		titleKeywords: titleKeywords,
		locations:     locations,
	}
}

// Match returns true if the job's title contains any title keyword and the
// job's location contains any location keyword. Empty keyword lists pass all.
func (f *TitleAndLocationFilter) Match(job model.Job) bool {
	titleLower := strings.ToLower(job.Title)
	locationLower := strings.ToLower(job.Location)

	if len(f.titleKeywords) > 0 {
		matched := false
		for _, kw := range f.titleKeywords {
			if strings.Contains(titleLower, strings.ToLower(kw)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if len(f.locations) > 0 {
		matched := false
		for _, loc := range f.locations {
			if strings.Contains(locationLower, strings.ToLower(loc)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}
