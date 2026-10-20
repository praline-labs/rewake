package modhost

import "net/http"

// sessionClosing hears the mod say the harness's session has ended and its
// process is ending, at session.end, and tells the run's Closing
// (docs/v2/shell.md). A second word changes nothing.
func (s *Server) sessionClosing(w http.ResponseWriter, r *http.Request) {
	var said ClosingRequest
	if status, err := decode(w, r, &said, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if said.Reason == "" {
		refuse(w, http.StatusBadRequest, "a closing names its reason")
		return
	}
	if !s.admittedLoad(said.LoadID) {
		refuse(w, http.StatusBadRequest, "no hello of this load was admitted in this run")
		return
	}
	if s.cfg.Closing != nil {
		s.cfg.Closing()
	}
	s.answer(w, r, Done{})
}
