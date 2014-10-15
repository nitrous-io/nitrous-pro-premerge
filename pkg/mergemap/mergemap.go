package mergemap

func Merge(from, to map[string]interface{}) {
	for k, v := range from {
		to[k] = v
	}
}
