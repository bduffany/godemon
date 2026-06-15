package godemon

func pointerTo[T any](val T) *T {
	return &val
}
