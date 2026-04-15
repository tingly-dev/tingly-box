package fetcher

// calcPercent returns used/limit * 100, capped at 100.
func calcPercent(used, limit float64) float64 {
	if limit <= 0 {
		return 0
	}
	p := (used / limit) * 100
	if p > 100 {
		return 100
	}
	return p
}
