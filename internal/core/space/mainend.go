package space

import "errors"

// MainEnding is the reason of a space whose main's harness said it has begun
// to end its process: the spaces' hosts end their workers from it, a moment
// before they would see main's run gone.
const MainEnding = "main's session ended"

// BeginClosingFor moves every open space of a main's run to closing with why,
// which each space's host reads at its next tick and ends its worker by the
// close sequence. It is best effort: a space it could not move is ended by its
// host when main's run is gone, as before. Main's end is final, so no work
// that would have gone on is stopped by it (docs/rules/space.md, SP1).
func BeginClosingFor(dir, mainRun, why string) error {
	records, _, err := List(dir)
	if err != nil {
		return err
	}
	var failed []error
	for _, record := range records {
		if record.State != Open || record.Main.Run != mainRun {
			continue
		}
		if _, err := BeginClosing(dir, record.ID, why); err != nil {
			failed = append(failed, err)
		}
	}
	return errors.Join(failed...)
}
