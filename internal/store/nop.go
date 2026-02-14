package store

import "time"

// NopStore is a no-op store used in dry-run mode. It never marks jobs as seen,
// so every job appears new on each poll.
type NopStore struct{}

func NewNopStore() *NopStore { return &NopStore{} }

func (s *NopStore) HasSeen(jobID string) (bool, error) { return false, nil }
func (s *NopStore) MarkSeen(jobID string) error        { return nil }
func (s *NopStore) Cleanup(olderThan time.Duration) error { return nil }
func (s *NopStore) IsEmpty() (bool, error)             { return false, nil }
