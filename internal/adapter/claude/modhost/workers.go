package modhost

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
)

// The workers' pane (docs/v2/workers-panel.md): the list of the spaces the
// run opened, a chosen worker's screen as a Raster's cells, its keys and its
// size, relayed to the space hosts through what the host showed the run.

// pane is the run's side of the workers' pane.
type pane struct {
	// view is what the host showed the run; nil until it does, and for a
	// run that opens no spaces.
	view adapter.SpaceView
	// open are the screens the mod is looking at, by space, each closed once
	// no request named it for idle.
	open map[string]*openView
	idle time.Duration
	hold time.Duration
}

type openView struct {
	view adapter.ScreenView
	last time.Time
}

// ShowSpaces takes what the host offers of the run's spaces.
func (s *Server) ShowSpaces(view adapter.SpaceView) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pane.view = view
}

// errNoWorker refuses a request naming no worker the pane may show.
var errNoWorker = errors.New("no worker of this run's spaces has that space, or its screen cannot be reached")

// screenOf is the open view of space, opened if none is: only a space the
// list shows is opened. Views no request named for idle are closed here.
// The list is read and the space host dialed outside the run's lock, which
// the heartbeat and the turns take.
func (s *Server) screenOf(space string) (adapter.ScreenView, error) {
	s.mu.Lock()
	w := &s.pane
	spaces := w.view
	if spaces == nil || s.closed {
		s.mu.Unlock()
		return nil, errNoWorker
	}
	now := time.Now()
	for id, open := range w.open {
		if id != space && now.Sub(open.last) > w.idleFor() {
			_ = open.view.Close()
			delete(w.open, id)
		}
	}
	if open := w.open[space]; open != nil {
		open.last = now
		s.mu.Unlock()
		return open.view, nil
	}
	s.mu.Unlock()
	listed, err := spaces.Workers()
	if err != nil || !slices.ContainsFunc(listed, func(worker adapter.SpaceWorker) bool { return worker.Space == space }) {
		return nil, errNoWorker
	}
	view, err := spaces.View(space)
	if err != nil {
		return nil, errNoWorker
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		_ = view.Close()
		return nil, errNoWorker
	}
	if open := w.open[space]; open != nil {
		// Another request opened it meanwhile: one view a space.
		_ = view.Close()
		open.last = now
		return open.view, nil
	}
	if w.open == nil {
		w.open = map[string]*openView{}
	}
	w.open[space] = &openView{view: view, last: now}
	return view, nil
}

// drop closes the view of space when it is still view: one whose connection
// ended is opened again by the next request.
func (s *Server) drop(space string, view adapter.ScreenView) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if open := s.pane.open[space]; open != nil && open.view == view {
		_ = view.Close()
		delete(s.pane.open, space)
	}
}

// closeViews closes every open view; called under mu at the run's close.
func (w *pane) closeViews() {
	for id, open := range w.open {
		_ = open.view.Close()
		delete(w.open, id)
	}
}

func (w *pane) idleFor() time.Duration {
	if w.idle > 0 {
		return w.idle
	}
	return ViewIdleSeconds * time.Second
}

func (w *pane) holdFor() time.Duration {
	if w.hold > 0 {
		return w.hold
	}
	return ScreenHoldSeconds * time.Second
}

// admittedRequest decodes a workers request and refuses one of a load the
// run did not admit; it answers false once it refused.
func (s *Server) admittedRequest(w http.ResponseWriter, r *http.Request, v any, load func() string) bool {
	if status, err := decode(w, r, v, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return false
	}
	if !s.admittedLoad(load()) {
		refuse(w, http.StatusConflict, errNoHello.Error())
		return false
	}
	s.mu.Lock()
	shown := s.pane.view != nil
	s.mu.Unlock()
	if !shown {
		refuse(w, http.StatusNotFound, "this run opens no spaces")
		return false
	}
	return true
}

func (s *Server) workersList(w http.ResponseWriter, r *http.Request) {
	var request WorkersRequest
	if !s.admittedRequest(w, r, &request, func() string { return request.LoadID }) {
		return
	}
	s.mu.Lock()
	view := s.pane.view
	s.mu.Unlock()
	listed, err := view.Workers()
	if err != nil {
		refuse(w, http.StatusInternalServerError, "the spaces could not be read: "+err.Error())
		return
	}
	answer := WorkersAnswer{Workers: []Worker{}}
	for _, worker := range listed {
		item := Worker{
			Space: worker.Space, Name: worker.Name, Session: worker.Session, Harness: worker.Harness, State: worker.State,
			Activity: optional(worker.Activity), Model: optional(worker.Model), Effort: optional(worker.Effort),
		}
		if worker.Percent >= 0 {
			percent := worker.Percent
			item.Percent = &percent
		}
		answer.Workers = append(answer.Workers, item)
	}
	s.answer(w, r, answer)
}

func (s *Server) workersScreen(w http.ResponseWriter, r *http.Request) {
	var request ScreenRequest
	if !s.admittedRequest(w, r, &request, func() string { return request.LoadID }) {
		return
	}
	view, err := s.screenOf(request.Space)
	if err != nil {
		refuse(w, http.StatusNotFound, err.Error())
		return
	}
	s.mu.Lock()
	hold := s.pane.holdFor()
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), hold)
	defer cancel()
	screen, err := view.Screen(ctx, uint64(max(request.After, 0)))
	if err != nil || screen.Lines == nil {
		s.drop(request.Space, view)
		refuse(w, http.StatusNotFound, errNoWorker.Error())
		return
	}
	s.answer(w, r, ScreenAnswer{Seq: int(screen.Seq), Columns: screen.Cols, Rows: screen.Rows, Cells: cells(screen), Ended: screen.Ended})
}

func (s *Server) workersKeys(w http.ResponseWriter, r *http.Request) {
	var request KeysRequest
	if !s.admittedRequest(w, r, &request, func() string { return request.LoadID }) {
		return
	}
	view, err := s.screenOf(request.Space)
	if err == nil {
		if err = view.Keys([]byte(request.Keys)); err != nil {
			s.drop(request.Space, view)
		}
	}
	if err != nil {
		refuse(w, http.StatusNotFound, errNoWorker.Error())
		return
	}
	s.answer(w, r, Done{})
}

func (s *Server) workersResize(w http.ResponseWriter, r *http.Request) {
	var request ResizeRequest
	if !s.admittedRequest(w, r, &request, func() string { return request.LoadID }) {
		return
	}
	if request.Columns < 1 || request.Rows < 1 || request.Columns > MaxScreenSide || request.Rows > MaxScreenSide {
		refuse(w, http.StatusBadRequest, "a size is 1 to maxScreenSide on each side")
		return
	}
	view, err := s.screenOf(request.Space)
	if err == nil {
		if err = view.Resize(request.Columns, request.Rows); err != nil {
			s.drop(request.Space, view)
		}
	}
	if err != nil {
		refuse(w, http.StatusNotFound, errNoWorker.Error())
		return
	}
	s.answer(w, r, Done{})
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
