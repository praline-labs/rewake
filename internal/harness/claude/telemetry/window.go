package telemetry

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// The auto-compact window: a limit a person may set below the model's own
// window, which the harness then treats as the window. Neither the status line
// nor session.measure carries it — both report the model's window and a percent
// of that — so the plugin reads what the harness reads and the collector puts
// the limit in the model window's place (docs/claude-telemetry.md).
//
// The rules below are the harness's own, read in the 2.1.280 binary
// (docs/research.md): the environment variable, then the --autocompact flag,
// then the settings key. What comes after those — remote configuration,
// experiments, a model's default — is not visible from here, and the model
// window stands as before.

const (
	// minimumWindow and maximumWindow bound every source of the limit.
	minimumWindow = 100000
	maximumWindow = 1000000
)

// Limit is what the plugin found the session configured with: the value of
// CLAUDE_CODE_AUTO_COMPACT_WINDOW and the settings key autoCompactWindow, each
// already read by the harness's rules, nil where it sets nothing. A Limit with
// both nil is an answer: the session has none of the two.
type Limit struct {
	Env      *int64 `json:"env,omitempty"`
	Settings *int64 `json:"settings,omitempty"`
}

// Autocompact is the --autocompact flag a session was launched with. Auto
// means the flag chose the harness's own window, which also sets the settings
// key aside.
type Autocompact struct {
	Set    bool
	Auto   bool
	Window int64
}

// envWindow reads the environment variable. An empty value is not set; a value
// that is no number, or not above zero, is ignored and the next source
// decides; a value above the maximum is cut to it, and one below the minimum
// raised to it.
func envWindow(raw string) *int64 {
	if raw == "" {
		return nil
	}
	value := parseCount(raw)
	if math.IsNaN(value) || value <= 0 {
		return nil
	}
	value = math.Max(minimumWindow, math.Min(value, maximumWindow))
	window := int64(value)
	return &window
}

// settingsWindow reads the settings key: a whole number within the bounds, or
// nothing — the harness's schema drops anything else.
func settingsWindow(value *float64) *int64 {
	if value == nil || *value != math.Trunc(*value) || *value < minimumWindow || *value > maximumWindow {
		return nil
	}
	window := int64(*value)
	return &window
}

// ParseAutocompact reads the flag's value: auto, or a count with an optional k
// or m, or a bare 100 to 1000 meaning thousands. The harness refuses to start
// on anything else, so a value it would refuse is not read here either.
func ParseAutocompact(raw string) Autocompact {
	text := strings.ToLower(strings.TrimFunc(raw, isScriptSpace))
	if text == "auto" {
		return Autocompact{Set: true, Auto: true}
	}
	var value float64
	switch {
	case strings.HasSuffix(text, "m"):
		value = leadingFloat(text) * 1e6
	case strings.HasSuffix(text, "k"):
		value = leadingFloat(text) * 1000
	default:
		value = parseCount(text)
		if value >= 100 && value <= 1000 {
			value *= 1000
		}
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < minimumWindow || value > maximumWindow {
		return Autocompact{}
	}
	return Autocompact{Set: true, Window: int64(math.Round(value))}
}

// The harness's reading of a count, ported one to one from its 2.1.280 bundle
// (the functions N and ac there): a form with an exponent, which must come out
// a whole number or reads as no number at all; digits grouped in threes by one
// separator used throughout; and otherwise the integer the text begins with,
// as JavaScript's parseInt reads it — so "300k" is 300 and "0.5" is 0.
var (
	exponent   = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)[eE][+-]?\d+$`)
	leadingInt = regexp.MustCompile(`^[+-]?\d+`)
	leadingNum = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?`)
)

// strictLength is the longest text the strict forms are tried on, counted in
// UTF-16 units as JavaScript counts a string's length.
const strictLength = 32

// parseCount is the harness's ac: NaN where it reads no number.
func parseCount(raw string) float64 {
	text := strings.TrimFunc(raw, isScriptSpace)
	if value, ok := strictCount(text); ok {
		return value
	}
	prefix := leadingInt.FindString(text)
	if prefix == "" {
		return math.NaN()
	}
	// A run of digits too long for a float reads as infinity, as in JavaScript.
	value, _ := strconv.ParseFloat(prefix, 64)
	return value
}

// strictCount is the harness's N: the exponent form and the grouped form, and
// false where the text is neither, for the lenient reading to take over.
func strictCount(text string) (float64, bool) {
	if len(utf16.Encode([]rune(text))) > strictLength {
		return 0, false
	}
	if exponent.MatchString(text) {
		value, _ := strconv.ParseFloat(text, 64)
		if math.IsInf(value, 0) || value != math.Trunc(value) {
			return math.NaN(), true
		}
		return value, true
	}
	if digits, ok := ungrouped(text); ok {
		value, _ := strconv.ParseFloat(digits, 64)
		return value, true
	}
	return 0, false
}

// ungrouped matches the harness's grouped form — a sign, one to three digits,
// then groups of exactly three, each after the same separator: an underscore,
// a comma, a space, a no-break space or a narrow no-break space — and returns
// the sign and digits alone.
func ungrouped(text string) (string, bool) {
	runes := []rune(text)
	var digits strings.Builder
	i := 0
	if i < len(runes) && (runes[i] == '+' || runes[i] == '-') {
		digits.WriteRune(runes[i])
		i++
	}
	lead := 0
	for i < len(runes) && isDigit(runes[i]) {
		digits.WriteRune(runes[i])
		i++
		lead++
	}
	if lead < 1 || lead > 3 || i == len(runes) || !strings.ContainsRune("_,\u00a0\u202f ", runes[i]) {
		return "", false
	}
	separator := runes[i]
	for i < len(runes) {
		if runes[i] != separator || len(runes)-i-1 < 3 {
			return "", false
		}
		for _, r := range runes[i+1 : i+4] {
			if !isDigit(r) {
				return "", false
			}
			digits.WriteRune(r)
		}
		i += 4
	}
	return digits.String(), true
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// isScriptSpace is what JavaScript's trim removes: the Unicode space
// separators, the ASCII whitespace controls, the byte-order mark and the line
// and paragraph separators — not the next-line control Go's TrimSpace also
// takes.
func isScriptSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', '\ufeff', '\u2028', '\u2029':
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// leadingFloat is the number a text begins with, as JavaScript's parseFloat
// reads it: NaN when it begins with none.
func leadingFloat(text string) float64 {
	prefix := leadingNum.FindString(text)
	if prefix == "" {
		return math.NaN()
	}
	value, _ := strconv.ParseFloat(prefix, 64)
	return value
}

// configured is the limit the session runs under, or nil for none this side
// can see: the environment first, then the flag — whose auto sets the key
// aside — then the key.
func configured(limit *Limit, flag Autocompact) *int64 {
	switch {
	case limit == nil:
		return nil
	case limit.Env != nil:
		return limit.Env
	case flag.Set && flag.Auto:
		return nil
	case flag.Set:
		return &flag.Window
	}
	return limit.Settings
}

// capped puts a limit in the model window's place where it is the smaller,
// and the percent with it. A context whose model window is unknown is left as
// it is: the limit only ever lowers a window, and there is none to lower.
func capped(context *Context, limit *int64) *Context {
	if context == nil || limit == nil || context.Window == nil || *limit >= *context.Window {
		return context
	}
	out := &Context{Used: context.Used, Window: limit}
	if context.Used != nil {
		percent := int(math.Round(float64(*context.Used) * 100 / float64(*limit)))
		out.Percent = &percent
	}
	return out
}
