package pi

func (harness) LaunchSettings(model, effort string) []string {
	var args []string
	if model != "" {
		args = append(args, "--model", model)
	}
	if effort != "" {
		args = append(args, "--thinking", effort)
	}
	return args
}
