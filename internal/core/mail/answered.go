package mail

// A wait is answered once its report is written and only then cleared
// (publishTurnContext). A run that dies between the two leaves a wait that
// names tasks already answered; a run resuming its conversation would take it
// over and answer them again under its own report's id. So the writing of a
// report first looks for the answer in the mailbox of whoever waits.

// answeredIDs names the messages a final report of the name — finished or
// failed, as stageOf counts them — has answered
// in the mailbox of to's run toEpoch, from any run of that name: a run that
// resumed a conversation reports on what an earlier one read.
func answeredIDs(dir, to, toEpoch, from string) (map[string]bool, error) {
	all, err := everywhere(dir, to)
	if err != nil {
		return nil, err
	}
	answered := map[string]bool{}
	for _, report := range all {
		kind := KindOf(report)
		if report.From != from || report.ToEpoch != toEpoch || kind != Finished && kind != Error {
			continue
		}
		for _, id := range report.InReplyTo {
			answered[id] = true
		}
	}
	return answered, nil
}

// answeredAlready says whether every message of ids has a final report of
// the name in the mailbox of to's run toEpoch.
func answeredAlready(dir, to, toEpoch, from string, ids []string) (bool, error) {
	answered, err := answeredIDs(dir, to, toEpoch, from)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if !answered[id] {
			return false, nil
		}
	}
	return len(ids) > 0, nil
}
