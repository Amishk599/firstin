package filter

import (
	"testing"

	"github.com/amishk599/firstin/internal/model"
)

func job(title, location string) model.Job {
	return model.Job{Title: title, Location: location}
}

func TestTitleAndLocationFilter_Match(t *testing.T) {
	tests := []struct {
		name                 string
		titleKeywords        []string
		titleExcludeKeywords []string
		locations            []string
		excludeLocations     []string
		job                  model.Job
		wantMatch            bool
	}{
		{
			name:          "matches both title and location",
			titleKeywords: []string{"software engineer", "backend"},
			locations:     []string{"United States", "Remote"},
			job:           job("Software Engineer", "Remote - US"),
			wantMatch:     true,
		},
		{
			name:          "title match but location miss",
			titleKeywords: []string{"software engineer"},
			locations:     []string{"United States", "Remote"},
			job:           job("Software Engineer", "London, UK"),
			wantMatch:     false,
		},
		{
			name:          "case insensitive matching",
			titleKeywords: []string{"FULLSTACK"},
			locations:     []string{"us"},
			job:           job("Fullstack Developer", "US Remote"),
			wantMatch:     true,
		},
		{
			name:          "no keywords match",
			titleKeywords: []string{"devops", "sre"},
			locations:     []string{"Remote"},
			job:           job("Frontend Engineer", "New York, NY"),
			wantMatch:     false,
		},
		{
			name:      "empty keyword lists pass all",
			job:       job("Any Role", "Anywhere"),
			wantMatch: true,
		},
		{
			name:                 "title matches include but hits exclude",
			titleKeywords:        []string{"software engineer"},
			titleExcludeKeywords: []string{"manager", "intern"},
			locations:            []string{"Remote"},
			job:                  job("Software Engineer Intern", "Remote"),
			wantMatch:            false,
		},
		{
			name:             "location matches include but hits exclude",
			titleKeywords:    []string{"software engineer"},
			locations:        []string{"CA"},
			excludeLocations: []string{"Canada", "Cairo"},
			job:              job("Software Engineer", "Toronto, Canada"),
			wantMatch:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewTitleAndLocationFilter(tt.titleKeywords, tt.titleExcludeKeywords, tt.locations, tt.excludeLocations)
			got := f.Match(tt.job)
			if got != tt.wantMatch {
				t.Errorf("Match() = %v, want %v", got, tt.wantMatch)
			}
		})
	}
}
