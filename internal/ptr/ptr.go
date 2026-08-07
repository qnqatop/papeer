package ptr

// Val dereferences a pointer, returning the zero value if nil.
func Val[T any, P *T](p P) T {
	if p != nil {
		return *p
	}
	var def T
	return def
}

// Ptr returns a pointer to t, or nil if t is the zero value.
func Ptr[T comparable](t T) *T {
	var def T
	if t == def {
		return nil
	}
	return &t
}
