package stream

// fileState tracks the current read offset for one tailed file path.
type fileState struct {
	offset int64
}
