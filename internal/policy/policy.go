package policy

import "time"

type Policy struct {
	ID      string
	Name    string
	Raw     string
	Updated time.Time
}

type Store struct {
	pol *Policy
}

func NewStore() *Store { return &Store{} }

func (s *Store) Get() *Policy {
	if s.pol == nil {
		return nil
	}
	return s.pol
}

func (s *Store) Set(p *Policy) { s.pol = p }
