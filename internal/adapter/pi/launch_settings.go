package pi

// LaunchSettings ignores context: pi has no flag for its window.
func (harness) LaunchSettings(model, effort, _ string) ([]string, error) {
	var args []string
	if model != "" {
		args = append(args, "--model", model)
	}
	if effort != "" {
		args = append(args, "--thinking", effort)
	}
	return args, nil
}

func (harness) AppliesContext() bool { return false }
