package brief

func roleText(id string) string {
	switch id {
	case "main":
		return "You are the main session: sessions you give work to report back to you when their turn ends, and your own turns are reported to nobody."
	case "write":
		return "When you finish work another session gave you, end your turn with the result as your final message and stop: rewake delivers that message to it. You can commit changes in the repository of your working directory."
	default:
		return "When you finish work another session gave you, end your turn with the result as your final message and stop: rewake delivers that message to it."
	}
}
