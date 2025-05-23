package transport

type ZeroWindowSizeError struct {
}

func (z ZeroWindowSizeError) Error() string {
	return "zero window size"
}
