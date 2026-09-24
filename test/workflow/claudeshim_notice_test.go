package workflow

// Reading the notice a delivery carries: the tagged block the adapter builds,
// and everything this column's session can know about the mail from it.

import (
	"fmt"
	"strconv"
	"strings"
)

// parseNotification reads the tagged block the adapter builds. A session sees
// exactly this text, so what the scenario may observe on this column is
// exactly what can be read out of it.
func parseNotification(content string) (claudeNotice, error) {
	var notice claudeNotice
	notice.TaskID = between(content, "<task-id>", "</task-id>")
	notice.Status = between(content, "<status>", "</status>")
	notice.Summary = strings.TrimSpace(unescapeNotice(between(content, "<summary>", "</summary>")))
	if notice.TaskID == "" || notice.Status == "" || notice.Summary == "" {
		return notice, fmt.Errorf("a notification without an id, a status or a summary: %s", firstLine(content))
	}
	first, preview, _ := strings.Cut(notice.Summary, "\n")
	notice.Preview = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(preview), "↳"))
	count, err := announcedCount(first)
	if err != nil {
		return notice, err
	}
	notice.Count = count
	return notice, nil
}

// announcedCount reads how many messages the notice says are waiting. It is
// the only thing this column knows about the membership of a group: the
// adapter names the group, not its members.
func announcedCount(line string) (int, error) {
	fields := strings.Fields(line)
	for index, field := range fields {
		if index+1 < len(fields) && strings.HasPrefix(fields[index+1], "new") {
			count, err := strconv.Atoi(field)
			if err == nil {
				return count, nil
			}
		}
	}
	return 0, fmt.Errorf("a notice that does not say how many messages are waiting: %q", line)
}

func between(text, opening, closing string) string {
	_, rest, found := strings.Cut(text, opening)
	if !found {
		return ""
	}
	inner, _, found := strings.Cut(rest, closing)
	if !found {
		return ""
	}
	return inner
}

// unescapeNotice undoes what the adapter escapes so a sender name cannot close
// a tag early.
func unescapeNotice(text string) string {
	return strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&").Replace(text)
}
