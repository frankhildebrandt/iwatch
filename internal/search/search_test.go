package search

import "testing"

func TestNextPrevIndex(t *testing.T) {
	hits := []int{2, 5, 9}
	if got := NextIndex(hits, 5); got != 9 {
		t.Fatalf("NextIndex() = %d", got)
	}
	if got := PrevIndex(hits, 5); got != 2 {
		t.Fatalf("PrevIndex() = %d", got)
	}
}
