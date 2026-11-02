package mirror

import (
	"encoding/json"
	"fmt"
)

// The model across an exec of its wrapper (docs/face.md, "A switch of the
// room's build"): the next image serves the same conversation, and a face
// that lost its connection with the old image reconnects and draws it as it
// was. The model leaves memory only for memory: the host carries what Hand
// answers in a memory-backed descriptor, never a file on disk.

// HandedVersion is the version of what Hand answers; Adopted takes only its
// own, and a switch of the room admits only a build that takes the same.
const HandedVersion = 1

type handedModel struct {
	Version int                       `json:"version"`
	Items   []handedEntry             `json:"items"`
	Parts   map[string]handedAssembly `json:"parts,omitempty"`
	Bytes   int                       `json:"bytes"`
	Dropped int                       `json:"dropped"`
	Turn    TurnState                 `json:"turn"`
	Mode    *PermissionMode           `json:"mode,omitempty"`
	Level   *ThinkingLevel            `json:"level,omitempty"`
	Born    int                       `json:"born"`
	Notices int                       `json:"notices"`
	Since   int                       `json:"since"`
	Worked  *handedWorked             `json:"worked,omitempty"`
	Retry   int                       `json:"retry"`
}

type handedEntry struct {
	Item   Item       `json:"item"`
	Text   handedClip `json:"text"`
	Output handedClip `json:"output"`
	Size   int        `json:"size"`
}

type handedAssembly struct {
	Turn  string     `json:"turn"`
	Block ItemKind   `json:"block"`
	Next  int        `json:"next"`
	Of    int        `json:"of"`
	Text  handedClip `json:"text"`
	Size  int        `json:"size"`
	Born  int        `json:"born"`
}

type handedClip struct {
	Head     string `json:"head"`
	Tail     string `json:"tail,omitempty"`
	HeadSize int    `json:"headSize"`
	TailSize int    `json:"tailSize,omitempty"`
	Cut      bool   `json:"cut,omitempty"`
	Max      int    `json:"max"`
}

type handedWorked struct {
	Took    int  `json:"took"`
	At      int  `json:"at"`
	Stopped bool `json:"stopped,omitempty"`
}

// Hand answers the model as the next image adopts it: its items and the
// settles being put together, with every count, so the bounds go on as they
// were. The faces, the inputs and the clock are the next image's own.
func (m *Model) Hand() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	handed := handedModel{
		Version: HandedVersion, Bytes: m.bytes, Dropped: m.dropped, Turn: m.turn, Mode: m.mode, Level: m.level,
		Born: m.born, Notices: m.notices, Since: m.since, Retry: m.retry, Parts: map[string]handedAssembly{},
	}
	for _, at := range m.order {
		handed.Items = append(handed.Items, handedEntry{Item: at.item, Text: handClip(at.text), Output: handClip(at.output), Size: at.size})
	}
	for id, a := range m.parts {
		handed.Parts[id] = handedAssembly{Turn: a.turn, Block: a.block, Next: a.next, Of: a.of, Text: handClip(a.text), Size: a.size, Born: a.born}
	}
	if w := m.worked; w != nil {
		handed.Worked = &handedWorked{Took: w.took, At: w.at, Stopped: w.stopped}
	}
	return json.Marshal(handed)
}

// Adopted is the model an earlier image handed over.
func Adopted(raw []byte) (*Model, error) {
	var handed handedModel
	if err := json.Unmarshal(raw, &handed); err != nil {
		return nil, fmt.Errorf("the face's model handed over cannot be read: %w", err)
	}
	if handed.Version != HandedVersion {
		return nil, fmt.Errorf("the face's model was handed over as version %d; this build takes %d", handed.Version, HandedVersion)
	}
	m := New()
	m.bytes, m.dropped, m.turn, m.mode, m.level = handed.Bytes, handed.Dropped, handed.Turn, handed.Mode, handed.Level
	m.born, m.notices, m.since, m.retry = handed.Born, handed.Notices, handed.Since, handed.Retry
	for _, e := range handed.Items {
		at := &entry{item: e.Item, text: adoptClip(e.Text), output: adoptClip(e.Output), size: e.Size}
		m.order = append(m.order, at)
		m.byID[at.item.ID] = at
	}
	for id, a := range handed.Parts {
		m.parts[id] = &assembly{turn: a.Turn, block: a.Block, next: a.Next, of: a.Of, text: adoptClip(a.Text), size: a.Size, born: a.Born}
	}
	if w := handed.Worked; w != nil {
		m.worked = &worked{took: w.Took, at: w.At, stopped: w.Stopped}
	}
	return m, nil
}

func handClip(c clip) handedClip {
	return handedClip{Head: c.head, Tail: c.tail, HeadSize: c.headSize, TailSize: c.tailSize, Cut: c.cut, Max: c.max}
}

func adoptClip(c handedClip) clip {
	return clip{head: c.Head, tail: c.Tail, headSize: c.HeadSize, tailSize: c.TailSize, cut: c.Cut, max: c.Max}
}
