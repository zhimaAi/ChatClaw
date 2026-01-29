package windows

import (
	"sync"

	"willchat/internal/errs"
	"willchat/internal/services/i18n"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	WindowSettings = "settings"
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
	app  *application.App
	i18n *i18n.Service
	mu   sync.RWMutex
	defs    map[string]WindowDefinition
	windows map[string]*application.WebviewWindow
}

func NewWindowService(app *application.App, i18nSvc *i18n.Service, defs []WindowDefinition) (*WindowService, error) {
	if app == nil {
		return nil, errs.NewI18n(i18nSvc, "error.app_required", nil)
	}
	if i18nSvc == nil {
		// 这里没法用 i18n，直接返回固定错误
		return nil, &errs.I18nError{Key: "error.i18n_required", Message: "i18n service is required"}
	}
	s := &WindowService{
		app:     app,
		i18n:    i18nSvc,
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
		return errs.NewI18n(s.i18n, "error.window_name_required", nil)
	}
	if def.CreateOptions == nil {
		return errs.NewI18nF(s.i18n, "error.window_create_options_required", nil, def.Name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.defs[def.Name]; exists {
		return errs.NewI18nF(s.i18n, "error.window_already_registered", nil, def.Name)
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
		return nil, errs.NewI18nF(s.i18n, "error.window_not_registered", nil, name)
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
		return errs.NewI18nF(s.i18n, "error.window_not_registered", nil, name)
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
		return errs.NewI18nF(s.i18n, "error.window_not_registered", nil, name)
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
		return false, errs.NewI18nF(s.i18n, "error.window_not_registered", nil, name)
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
