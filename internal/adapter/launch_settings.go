package adapter

// LaunchSettingsHarness translates an alias's portable settings into this
// harness's flags. Context is descriptive only and never reaches this method.
type LaunchSettingsHarness interface {
	LaunchSettings(model, effort string) []string
}
