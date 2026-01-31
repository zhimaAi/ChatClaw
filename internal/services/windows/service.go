package windows

import (
	"sync"

	"willchat/internal/errs"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	WindowWinsnap = "winsnap"
)

type WindowInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Created bool   `json:"created"`
	Visible bool   `json:"visible"`
}

type WindowDefinition struct {
	Name          string
	CreateOptions func() application.WebviewWindowOptions
	FocusOnShow   bool
}

type WindowService struct {
	app     *application.App
	mu      sync.RWMutex
	defs    map[string]WindowDefinition
	windows map[string]*application.WebviewWindow
}

func NewWindowService(app *application.App, defs []WindowDefinition) (*WindowService, error) {
	if app == nil {
		return nil, errs.New("error.app_required")
	}
	s := &WindowService{
		app:     app,
		defs:    make(map[string]WindowDefinition),
		windows: make(map[string]*application.WebviewWindow),
	}
	for _, def := range defs {
		if err := s.register(def); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *WindowService) register(def WindowDefinition) error {
	if def.Name == "" {
		return errs.New("error.window_name_required")
	}
	if def.CreateOptions == nil {
		return errs.Newf("error.window_create_options_required", map[string]any{"Name": def.Name})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.defs[def.Name]; exists {
		return errs.Newf("error.window_already_registered", map[string]any{"Name": def.Name})
	}
	s.defs[def.Name] = def
	return nil
}

func (s *WindowService) ensure(name string) (*application.WebviewWindow, error) {
	s.mu.RLock()
	if existing := s.windows[name]; existing != nil {
		s.mu.RUnlock()
		return existing, nil
	}
	def, ok := s.defs[name]
	s.mu.RUnlock()
	if !ok {
		return nil, errs.Newf("error.window_not_registered", map[string]any{"Name": name})
	}

	options := def.CreateOptions()
	if options.Name == "" {
		options.Name = name
	}

	w := s.app.Window.NewWithOptions(options)

	w.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		s.mu.Lock()
		delete(s.windows, name)
		s.mu.Unlock()
	})

	s.mu.Lock()
	s.windows[name] = w
	s.mu.Unlock()

	return w, nil
}

func (s *WindowService) List() []WindowInfo {
	type item struct {
		name string
		def  WindowDefinition
		w    *application.WebviewWindow
	}

	s.mu.RLock()
	items := make([]item, 0, len(s.defs))
	for name, def := range s.defs {
		items = append(items, item{name: name, def: def, w: s.windows[name]})
	}
	s.mu.RUnlock()

	result := make([]WindowInfo, 0, len(items))
	for _, it := range items {
		info := WindowInfo{
			Name:    it.name,
			Created: it.w != nil,
			Visible: false,
		}

		opts := it.def.CreateOptions()
		info.Title = opts.Title
		info.URL = opts.URL

		if it.w != nil {
			info.Visible = it.w.IsVisible()
		}
		result = append(result, info)
	}
	return result
}

func (s *WindowService) Show(name string) error {
	s.mu.RLock()
	def, ok := s.defs[name]
	s.mu.RUnlock()
	if !ok {
		return errs.Newf("error.window_not_registered", map[string]any{"Name": name})
	}

	w, err := s.ensure(name)
	if err != nil {
		return err
	}
	w.Show()
	if def.FocusOnShow {
		w.Focus()
	}
	return nil
}

func (s *WindowService) Close(name string) error {
	s.mu.Lock()
	_, registered := s.defs[name]
	w := s.windows[name]
	delete(s.windows, name)
	s.mu.Unlock()
	if !registered {
		return errs.Newf("error.window_not_registered", map[string]any{"Name": name})
	}
	if w == nil {
		return nil
	}

	w.Close()
	return nil
}

func (s *WindowService) IsVisible(name string) (bool, error) {
	s.mu.RLock()
	_, registered := s.defs[name]
	w := s.windows[name]
	s.mu.RUnlock()

	if !registered {
		return false, errs.Newf("error.window_not_registered", map[string]any{"Name": name})
	}
	if w == nil {
		return false, nil
	}
	return w.IsVisible(), nil
}

func (s *WindowService) SetVisible(name string, visible bool) (bool, error) {
	if visible {
		if err := s.Show(name); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := s.Close(name); err != nil {
		return false, err
	}
	return false, nil
}

func (s *WindowService) Toggle(name string) (bool, error) {
	current, err := s.IsVisible(name)
	if err != nil {
		return false, err
	}
	return s.SetVisible(name, !current)
}
