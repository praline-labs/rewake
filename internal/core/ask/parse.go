package ask

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The choices of a permission, as main types them, in the order of the
// permission question's options: allow this call, allow this exact call for
// the worker's run, until its mod reloads, refuse with a reason.
const (
	ChoiceOnce    = "once"
	ChoiceSession = "session"
	ChoiceRefuse  = "refuse"
)

var permissionChoices = []string{ChoiceOnce, ChoiceSession, ChoiceRefuse}

var numbers = regexp.MustCompile(`^[0-9]+(,[0-9]+)*$`)

// Parse reads main's answer to an offer from the words it typed. A
// permission takes once, session, or refuse with an optional reason after
// it. A question takes one word per question — a single one takes every word
// as one — each the numbers of its options, several joined by commas where
// the question allows several, or any other text, which is the answer typed
// under Other.
func Parse(offer Offer, words []string) (Answer, error) {
	if len(words) == 0 {
		return Answer{}, errors.New("no choice given")
	}
	if offer.Kind == Permission {
		return parsePermission(offer, words)
	}
	if len(offer.Questions) == 1 {
		words = []string{strings.Join(words, " ")}
	}
	if len(words) != len(offer.Questions) {
		return Answer{}, fmt.Errorf("the request asks %d questions and the answer gives %d; quote each answer as one word", len(offer.Questions), len(words))
	}
	answer := Answer{Answers: map[string]string{}}
	for index, prompt := range offer.Questions {
		text, err := choose(prompt, strings.TrimSpace(words[index]))
		if err != nil {
			return Answer{}, fmt.Errorf("question %d: %w", index+1, err)
		}
		answer.Answers[prompt.Text] = text
	}
	return answer, nil
}

func parsePermission(offer Offer, words []string) (Answer, error) {
	if len(offer.Questions) != 1 || len(offer.Questions[0].Options) != len(permissionChoices) {
		return Answer{}, errors.New("the permission request does not carry its three choices")
	}
	index := -1
	for i, choice := range permissionChoices {
		if strings.EqualFold(words[0], choice) {
			index = i
		}
	}
	if index < 0 {
		return Answer{}, fmt.Errorf("%q is not a choice of a permission: once, session or refuse", words[0])
	}
	reason := strings.TrimSpace(strings.Join(words[1:], " "))
	if reason != "" && permissionChoices[index] != ChoiceRefuse {
		return Answer{}, fmt.Errorf("only refuse takes a reason; %s takes nothing after it", permissionChoices[index])
	}
	prompt := offer.Questions[0]
	return Answer{Answers: map[string]string{prompt.Text: prompt.Options[index].Label}, Reason: reason}, nil
}

// choose is one question's answer: the labels of the numbers given, or the
// text itself.
func choose(prompt Prompt, word string) (string, error) {
	if word == "" {
		return "", errors.New("the answer is empty")
	}
	if !numbers.MatchString(word) {
		return word, nil
	}
	parts := strings.Split(word, ",")
	if len(parts) > 1 && !prompt.Multi {
		return "", errors.New("the question takes one option")
	}
	var labels []string
	seen := map[int]bool{}
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > len(prompt.Options) {
			return "", fmt.Errorf("%s is not an option: they are 1 to %d", part, len(prompt.Options))
		}
		if !seen[n] {
			seen[n] = true
			labels = append(labels, prompt.Options[n-1].Label)
		}
	}
	return strings.Join(labels, ", "), nil
}
