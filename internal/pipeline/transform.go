package pipeline

// FindTransformer returns the first transformer that can handle the input.
func FindTransformer(transformers []Transformer, input *RawInput) Transformer {
	for _, t := range transformers {
		if t.CanHandle(input) {
			return t
		}
	}
	return nil
}
