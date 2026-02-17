package filter

import (
	"strings"

	"github.com/amishk599/firstin/internal/model"
)

// TitleAndLocationFilter matches jobs whose title contains any of the title
// keywords and whose location contains any of the location keywords.
// It also rejects jobs whose title matches any exclude keyword or whose
// location matches any exclude location.
// Matching is case-insensitive. Empty keyword lists are treated as "match all".
type TitleAndLocationFilter struct {
	titleKeywords        []string
	titleExcludeKeywords []string
	locations            []string
	excludeLocations     []string
}

// NewTitleAndLocationFilter returns a filter that requires both a title keyword
// match and a location keyword match (case-insensitive substring), while
// rejecting titles or locations that match any exclusion keyword.
func NewTitleAndLocationFilter(titleKeywords, titleExcludeKeywords, locations, excludeLocations []string) *TitleAndLocationFilter {
	return &TitleAndLocationFilter{
		titleKeywords:        titleKeywords,
		titleExcludeKeywords: titleExcludeKeywords,
		locations:            locations,
		excludeLocations:     excludeLocations,
	}
}

// Match returns true if the job's title contains any title keyword (and none of
// the exclude keywords) and the job's location contains any location keyword
// (and none of the exclude locations). Empty keyword lists pass all.
func (f *TitleAndLocationFilter) Match(job model.Job) bool {
	titleLower := strings.ToLower(job.Title)
	locationLower := strings.ToLower(job.Location)

	// Title must match at least one include keyword (if any specified)
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

	// Title must NOT match any exclude keyword
	for _, kw := range f.titleExcludeKeywords {
		if strings.Contains(titleLower, strings.ToLower(kw)) {
			return false
		}
	}

	// Location must match at least one include location (if any specified)
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

	// Location must NOT match any exclude location
	for _, loc := range f.excludeLocations {
		if strings.Contains(locationLower, strings.ToLower(loc)) {
			return false
		}
	}

	return true
}
