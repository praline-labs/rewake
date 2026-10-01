package wrap

// noWriters is the check of a launch that finds no earlier-build writer.
func noWriters(string, string) error { return nil }
