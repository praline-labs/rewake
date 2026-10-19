package adapter

// LaunchSettingsHarness translates an alias's portable settings into this
// harness's flags. Context becomes a flag only where AppliesContext says the
// harness has one; elsewhere it is shown and never reaches the arguments.
type LaunchSettingsHarness interface {
	LaunchSettings(model, effort, context string) ([]string, error)
	AppliesContext() bool
}
