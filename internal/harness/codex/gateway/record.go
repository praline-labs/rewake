package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Record is an explicit allowlist. Payload bodies and live completion text are
// deliberately absent, even though the latter may reach the completion callback.
type Record struct {
	BindingThread     string   `json:"bindingThread,omitempty"`
	StartupFork       bool     `json:"startupFork,omitempty"`
	DetachStatus      string   `json:"detachStatus,omitempty"`
	ReadSerial        uint64   `json:"readSerial"`
	ReadPhase         string   `json:"readPhase"`
	ReadClosedBy      string   `json:"readClosedBy,omitempty"`
	ReadShape         string   `json:"includeTurnsShape,omitempty"`
	ParamsShape       string   `json:"paramsShape,omitempty"`
	ReadContext       string   `json:"readContext,omitempty"`
	LoadedValid       bool     `json:"loadedListValid,omitempty"`
	LoadedTags        []string `json:"loadedThreadTags,omitempty"`
	Direction         string   `json:"direction"`
	Method            string   `json:"method"`
	IDClass           string   `json:"idClass,omitempty"`
	RequestTag        string   `json:"requestTag,omitempty"`
	Source            string   `json:"source,omitempty"`
	Turn              string   `json:"turn,omitempty"`
	DirectKnown       bool     `json:"directKnown,omitempty"`
	Direct            bool     `json:"direct,omitempty"`
	PrimaryConnection bool     `json:"primaryConnection"`
	MetadataRead      bool     `json:"metadataOnlyRead,omitempty"`
	Refused           bool     `json:"refused,omitempty"`
	IDKind            string   `json:"idKind,omitempty"`
	Thread            string   `json:"thread,omitempty"`
	Status            string   `json:"status,omitempty"`
	ConfigPresent     bool     `json:"configPresent,omitempty"`
	RootsPresent      bool     `json:"rootsPresent,omitempty"`
	Generation        uint64   `json:"generation"`
	Connection        uint64   `json:"connection"`
	Ready             bool     `json:"ready"`
	Reason            string   `json:"reason"`
}

func (c *connection) record(direction string, m meta) {
	if c.owner.cfg.Record == nil {
		return
	}
	method := m.method
	for _, ch := range method {
		allowed := ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("/_-.$", ch)
		if !allowed {
			method = "invalid-method-shape"
			break
		}
	}
	status := ""
	switch m.status {
	case "idle", "active", "notLoaded", "systemError", "completed", "failed", "interrupted":
		status = m.status
	}
	idKind := ""
	if m.id != "" {
		idKind = "string"
		if m.numeric {
			idKind = "numeric"
		}
	}
	idClass := idKind
	if uuidID(m.idText) {
		idClass = "uuid"
	}
	tag := ""
	source := ""
	if m.id != "" {
		hash := sha256.Sum256([]byte(m.id))
		tag = hex.EncodeToString(hash[:8])
	}
	switch {
	case strings.HasPrefix(m.idText, "startup-thread-start-"):
		idClass = "startup"
	case strings.HasPrefix(m.idText, "tui-dynamic-"):
		idClass = "dynamic"
	case strings.HasPrefix(m.idText, "temporary-"):
		idClass = "temporary"
	}
	switch m.source {
	case "user", "feature:system":
		source = m.source
	}
	var tags []string
	for _, id := range m.loaded {
		h := sha256.Sum256([]byte(id))
		tags = append(tags, hex.EncodeToString(h[:8]))
	}
	detachStatus := ""
	if detached(m) {
		detachStatus = m.detachStatus
	}
	c.owner.cfg.Record(Record{BindingThread: c.state.Thread, StartupFork: m.startupFork, DetachStatus: detachStatus, ReadSerial: c.state.readSerial, ReadPhase: c.state.readPhase(), ReadClosedBy: c.state.readClosedBy, ReadShape: m.readShape, ParamsShape: m.paramsShape, ReadContext: m.readClass, LoadedValid: m.loadedValid, LoadedTags: tags, PrimaryConnection: c.owner.owns(c), MetadataRead: m.method == "thread/read" && metadataRead(m), IDClass: idClass, RequestTag: tag, Source: source, Turn: m.turn, DirectKnown: m.directKnown, Direct: m.direct, Refused: m.failure, Direction: direction, Method: method, IDKind: idKind, Thread: m.thread, Status: status, ConfigPresent: m.config, RootsPresent: m.roots, Generation: c.state.Generation, Connection: c.state.Connection, Ready: c.state.Ready, Reason: c.state.Reason})
}
