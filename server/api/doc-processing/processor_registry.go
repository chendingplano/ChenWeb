package docprocessing

var optionalProductionProcessorOrder = selectableProductionProcessorOrder()

func OptionalProductionProcessorNames() []string {
	return append([]string(nil), optionalProductionProcessorOrder...)
}

func CanonicalOptionalProductionProcessor(raw string) (string, bool) {
	name := normalizeRuntimeName(raw)
	for _, candidate := range optionalProductionProcessorOrder {
		if candidate == name {
			return candidate, true
		}
	}
	return "", false
}
