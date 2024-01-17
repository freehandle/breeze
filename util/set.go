package util

type Set[T comparable] map[T]struct{}

func SetFromSlice[T comparable](slice []T) map[T]struct{} {
	set := make(map[T]struct{})
	for _, item := range slice {
		set[item] = struct{}{}
	}
	return set
}
