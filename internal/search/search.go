package search

func NextIndex(hits []int, current int) int {
	if len(hits) == 0 {
		return -1
	}
	for _, hit := range hits {
		if hit > current {
			return hit
		}
	}
	return hits[0]
}

func PrevIndex(hits []int, current int) int {
	if len(hits) == 0 {
		return -1
	}
	for idx := len(hits) - 1; idx >= 0; idx-- {
		if hits[idx] < current {
			return hits[idx]
		}
	}
	return hits[len(hits)-1]
}
