package modhost

import "net/http"

// sessionClosing hears the mod say the harness has begun to end its process, at
// /exit and at session.end, and tells the run's Closing (docs/v2/shell.md).
// Measured on 2.1.302: /exit's command.run fires before the harness leaves
// its screen, session.end some 60 ms after, so the mod tells at both and a
// second word changes nothing.
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
