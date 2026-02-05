package multiask

import (
	"fmt"
	"sync"
	"time"

	"willchat/pkg/webviewpanel"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// PanelInfo 面板信息
type PanelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	URL         string `json:"url"`
	Visible     bool   `json:"visible"`
}

// PanelBounds 面板位置和大小
type PanelBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// MultiaskService 多问服务（管理多个 AI WebView 面板）
type MultiaskService struct {
	app         *application.App
	window      *application.WebviewWindow
	manager     *webviewpanel.PanelManager
	panels      map[string]*webviewpanel.WebviewPanel
	panelsLock  sync.RWMutex
	initialized bool
	initLock    sync.Mutex
}

// NewMultiaskService 创建多问服务
func NewMultiaskService(app *application.App, window *application.WebviewWindow) *MultiaskService {
	return &MultiaskService{
		app:    app,
		window: window,
		panels: make(map[string]*webviewpanel.WebviewPanel),
	}
}

// Initialize 初始化面板管理器（需要在主窗口创建后调用）
func (s *MultiaskService) Initialize(windowTitle string) error {
	s.initLock.Lock()
	defer s.initLock.Unlock()

	if s.initialized {
		s.app.Logger.Info("[MultiaskService] Already initialized")
		return nil
	}

	s.app.Logger.Info("[MultiaskService] Initializing...")

	// 方法1：直接从窗口获取句柄（推荐）
	var hwnd uintptr
	if s.window != nil {
		// 等待窗口完全创建，带重试机制
		maxRetries := 20
		for i := 0; i < maxRetries; i++ {
			hwnd = uintptr(s.window.NativeWindow())
			if hwnd != 0 {
				break
			}
			s.app.Logger.Info("[MultiaskService] Native window not ready, retrying...", "attempt", i+1)
			<-time.After(250 * time.Millisecond)
		}
	}

	// 方法2：如果直接获取失败，尝试通过标题查找
	if hwnd == 0 && windowTitle != "" {
		s.app.Logger.Info("[MultiaskService] Trying to find window by title...", "title", windowTitle)
		maxRetries := 10
		for i := 0; i < maxRetries; i++ {
			hwnd = webviewpanel.FindWindowByTitleContains(windowTitle)
			if hwnd != 0 {
				break
			}
			s.app.Logger.Info("[MultiaskService] Window not found by title, retrying...", "attempt", i+1)
			<-time.After(500 * time.Millisecond)
		}
	}

	if hwnd == 0 {
		s.app.Logger.Error("[MultiaskService] Cannot get window handle")
		return fmt.Errorf("cannot get window handle")
	}

	s.app.Logger.Info("[MultiaskService] Got window handle", "hwnd", hwnd)

	// 创建面板管理器
	s.manager = webviewpanel.NewPanelManager(hwnd, true)
	s.manager.SetDispatchSync(application.InvokeSync)
	s.initialized = true

	s.app.Logger.Info("[MultiaskService] Initialized successfully")
	return nil
}

// CreatePanel 创建 WebView 面板
func (s *MultiaskService) CreatePanel(id, name, displayName, url string, bounds PanelBounds) error {
	s.app.Logger.Info("[MultiaskService] CreatePanel called",
		"id", id,
		"name", name,
		"url", url,
		"bounds", bounds,
	)

	if !s.initialized {
		s.app.Logger.Error("[MultiaskService] Service not initialized")
		return fmt.Errorf("service not initialized, call Initialize first")
	}

	s.panelsLock.Lock()
	defer s.panelsLock.Unlock()

	// 检查面板是否已存在
	if _, exists := s.panels[id]; exists {
		s.app.Logger.Warn("[MultiaskService] Panel already exists", "id", id)
		return fmt.Errorf("panel %s already exists", id)
	}

	// 创建面板
	visible := true
	s.app.Logger.Info("[MultiaskService] Creating panel...",
		"id", id,
		"x", bounds.X,
		"y", bounds.Y,
		"width", bounds.Width,
		"height", bounds.Height,
	)

	panel := s.manager.NewPanel(webviewpanel.WebviewPanelOptions{
		Name:    id,
		X:       bounds.X,
		Y:       bounds.Y,
		Width:   bounds.Width,
		Height:  bounds.Height,
		URL:     url,
		Visible: &visible,
		ZIndex:  1,
	})

	s.panels[id] = panel
	s.app.Logger.Info("[MultiaskService] Panel created successfully", "id", id)
	return nil
}

// UpdatePanelBounds 更新面板位置和大小
func (s *MultiaskService) UpdatePanelBounds(id string, bounds PanelBounds) error {
	s.panelsLock.RLock()
	panel, exists := s.panels[id]
	s.panelsLock.RUnlock()

	if !exists {
		return fmt.Errorf("panel %s not found", id)
	}

	panel.SetBounds(webviewpanel.Rect{
		X:      bounds.X,
		Y:      bounds.Y,
		Width:  bounds.Width,
		Height: bounds.Height,
	})
	return nil
}

// ShowPanel 显示面板
func (s *MultiaskService) ShowPanel(id string) error {
	s.panelsLock.RLock()
	panel, exists := s.panels[id]
	s.panelsLock.RUnlock()

	if !exists {
		return fmt.Errorf("panel %s not found", id)
	}

	panel.Show()
	return nil
}

// HidePanel 隐藏面板
func (s *MultiaskService) HidePanel(id string) error {
	s.panelsLock.RLock()
	panel, exists := s.panels[id]
	s.panelsLock.RUnlock()

	if !exists {
		return fmt.Errorf("panel %s not found", id)
	}

	panel.Hide()
	return nil
}

// DestroyPanel 销毁面板
func (s *MultiaskService) DestroyPanel(id string) error {
	s.panelsLock.Lock()
	panel, exists := s.panels[id]
	if exists {
		delete(s.panels, id)
	}
	s.panelsLock.Unlock()

	if !exists {
		return fmt.Errorf("panel %s not found", id)
	}

	panel.Destroy()
	return nil
}

// DestroyAllPanels 销毁所有面板
func (s *MultiaskService) DestroyAllPanels() {
	s.panelsLock.Lock()
	panels := make([]*webviewpanel.WebviewPanel, 0, len(s.panels))
	for _, panel := range s.panels {
		panels = append(panels, panel)
	}
	s.panels = make(map[string]*webviewpanel.WebviewPanel)
	s.panelsLock.Unlock()

	for _, panel := range panels {
		panel.Destroy()
	}
}

// NavigatePanel 导航面板到新 URL
func (s *MultiaskService) NavigatePanel(id, url string) error {
	s.panelsLock.RLock()
	panel, exists := s.panels[id]
	s.panelsLock.RUnlock()

	if !exists {
		return fmt.Errorf("panel %s not found", id)
	}

	panel.SetURL(url)
	return nil
}

// RefreshPanel 刷新面板
func (s *MultiaskService) RefreshPanel(id string) error {
	s.panelsLock.RLock()
	panel, exists := s.panels[id]
	s.panelsLock.RUnlock()

	if !exists {
		return fmt.Errorf("panel %s not found", id)
	}

	panel.Reload()
	return nil
}

// ExecuteJS 在面板中执行 JavaScript
func (s *MultiaskService) ExecuteJS(id, js string) error {
	s.panelsLock.RLock()
	panel, exists := s.panels[id]
	s.panelsLock.RUnlock()

	if !exists {
		return fmt.Errorf("panel %s not found", id)
	}

	panel.ExecJS(js)
	return nil
}

// SendMessageToPanel 向面板发送消息（通过 JavaScript 注入到输入框并自动发送）
// 这个方法会尝试查找常见 AI 网站的输入框并填充内容，然后点击发送按钮
func (s *MultiaskService) SendMessageToPanel(id, message string) error {
	s.panelsLock.RLock()
	panel, exists := s.panels[id]
	s.panelsLock.RUnlock()

	if !exists {
		return fmt.Errorf("panel %s not found", id)
	}

	// JavaScript 代码：尝试查找并填充输入框，然后点击发送按钮
	// 支持多种常见 AI 网站的输入框和发送按钮选择器
	js := fmt.Sprintf(`
(function() {
    const message = %q;
    
    // 检测当前网站
    const hostname = window.location.hostname;
    const isChatGPT = hostname.includes('chatgpt.com') || hostname.includes('chat.openai.com');
    const isDoubao = hostname.includes('doubao.com');
    const isQwen = hostname.includes('qianwen.com') || hostname.includes('tongyi.aliyun.com');
    const isClaude = hostname.includes('claude.ai');
    const isDeepSeek = hostname.includes('deepseek.com');
    
    console.log('[WillChat] Detected site:', { hostname, isChatGPT, isDoubao, isQwen, isClaude, isDeepSeek });
    
    // 根据网站选择不同的输入框选择器
    let inputSelectors = [];
    let sendButtonSelectors = [];
    
    if (isChatGPT) {
        inputSelectors = [
            '#prompt-textarea',
            'div[id="prompt-textarea"]',
            'textarea[data-id="root"]',
            'div[contenteditable="true"][id="prompt-textarea"]',
            'div[contenteditable="true"]',
        ];
        sendButtonSelectors = [
            'button[data-testid="send-button"]',
            'button[data-testid="fruitjuice-send-button"]',
            'button[aria-label*="Send"]',
            'button[class*="send"]',
        ];
    } else if (isDoubao) {
        inputSelectors = [
            // 豆包的输入框
            '[class*="chat-input"] textarea',
            'textarea[class*="semi-input-textarea"]',
            'div[class*="inputContent"] textarea',
            'textarea[placeholder*="输入"]',
            'textarea[placeholder*="发送"]',
            'textarea',
        ];
        sendButtonSelectors = [
            // 豆包的发送按钮 - 通常是一个带有 SVG 的按钮或 div
            '[class*="send"][class*="btn"]',
            '[class*="sendBtn"]',
            '[class*="send-btn"]',
            'div[class*="opera"] button',
            '[class*="inputFooter"] button',
            'button[class*="primary"]',
        ];
    } else if (isQwen) {
        inputSelectors = [
            // 通义千问的输入框
            'textarea[class*="chatInput"]',
            '#chat-input textarea',
            'textarea[placeholder*="向千问提问"]',
            'textarea[placeholder*="输入"]',
            '[class*="ChatInput"] textarea',
            'textarea',
        ];
        sendButtonSelectors = [
            // 通义千问的发送按钮
            '[class*="sendBtn"]',
            '[class*="send-btn"]',
            'button[class*="ChatInputBtn"]',
            '[class*="operateBtn"]',
            '[class*="ChatInput"] button',
        ];
    } else if (isClaude) {
        inputSelectors = [
            'div[contenteditable="true"]',
            'div[aria-label*="Write"]',
            'p[data-placeholder]',
        ];
        sendButtonSelectors = [
            'button[aria-label="Send Message"]',
            'button[aria-label*="Send"]',
        ];
    } else if (isDeepSeek) {
        inputSelectors = [
            // DeepSeek 输入框
            'textarea#chat-input',
            'textarea[id*="chat-input"]',
            '#chat-input',
            'textarea[class*="ds-input"]',
            'div[class*="chat-input"] textarea',
            '[class*="InputArea"] textarea',
            'textarea[placeholder*="发消息"]',
            'textarea[placeholder*="输入"]',
            'textarea[placeholder*="Ask"]',
            'textarea[placeholder*="Message"]',
            'textarea',
        ];
        sendButtonSelectors = [
            // DeepSeek 发送按钮
            'div[class*="f6d670"] button', // 常见的发送按钮容器
            'button[class*="ds-button"]',
            '[class*="InputArea"] button',
            'div[class*="chat-input"] button',
            '[class*="send"] button',
            'button:has(svg)',
        ];
    } else {
        // 通用选择器
        inputSelectors = [
            'textarea[placeholder*="输入"]',
            'textarea[placeholder*="问"]',
            'textarea[placeholder*="消息"]',
            'textarea[placeholder*="message"]',
            'textarea[placeholder*="Message"]',
            'div[contenteditable="true"]',
            'textarea',
        ];
        sendButtonSelectors = [
            'button[type="submit"]',
            'button[class*="send"]',
            'button[class*="submit"]',
        ];
    }
    
    // 查找输入框
    let input = null;
    for (const selector of inputSelectors) {
        try {
            input = document.querySelector(selector);
            if (input) {
                console.log('[WillChat] Found input with selector:', selector);
                break;
            }
        } catch (e) {}
    }
    
    if (!input) {
        console.warn('[WillChat] No input element found');
        return;
    }
    
    // 填充输入框
    const fillInput = () => {
        if (input.tagName === 'TEXTAREA' || input.tagName === 'INPUT') {
            // 使用原生 setter 设置值（绕过 React/Vue 的状态管理）
            const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
                window.HTMLTextAreaElement.prototype, 'value'
            )?.set || Object.getOwnPropertyDescriptor(
                window.HTMLInputElement.prototype, 'value'
            )?.set;
            
            if (nativeInputValueSetter) {
                nativeInputValueSetter.call(input, message);
            } else {
                input.value = message;
            }
            
            // 触发各种事件以确保框架检测到变化
            input.dispatchEvent(new Event('input', { bubbles: true, cancelable: true }));
            input.dispatchEvent(new Event('change', { bubbles: true, cancelable: true }));
            input.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true }));
            input.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true }));
        } else if (input.contentEditable === 'true' || input.isContentEditable) {
            // ChatGPT 使用 contenteditable div
            input.focus();
            
            // 清空现有内容
            input.textContent = '';
            
            // 创建文本节点并插入
            input.appendChild(document.createTextNode(message));
        
            
            // 触发 input 事件
            input.dispatchEvent(new InputEvent('input', { 
                bubbles: true, 
                cancelable: true,
                inputType: 'insertText',
                data: message
            }));
            
            // 对于 ChatGPT，还需要触发一些额外的事件
            if (isChatGPT) {
                input.dispatchEvent(new Event('beforeinput', { bubbles: true }));
                input.dispatchEvent(new Event('input', { bubbles: true }));
            }
        }
        input.focus();
        console.log('[WillChat] Message injected successfully');
    };
    
    fillInput();
    
    // 延迟后查找并点击发送按钮
    setTimeout(() => {
        let sendButton = null;
        
        // 尝试各种发送按钮选择器
        for (const selector of sendButtonSelectors) {
            try {
                const btns = document.querySelectorAll(selector);
                for (const btn of btns) {
                    if (!btn.disabled && btn.offsetParent !== null) {
                        sendButton = btn;
                        console.log('[WillChat] Found send button with selector:', selector);
                        break;
                    }
                }
                if (sendButton) break;
            } catch (e) {}
        }
        
        // 如果没找到，尝试在输入框父元素附近查找按钮
        if (!sendButton) {
            let parent = input.closest('form') || input.parentElement;
            for (let i = 0; i < 5 && parent; i++) {
                const buttons = parent.querySelectorAll('button, div[role="button"], [class*="send"], [class*="submit"]');
                for (const btn of buttons) {
                    const isVisible = btn.offsetParent !== null;
                    const isNotDisabled = !btn.disabled && !btn.classList.contains('disabled');
                    const looksLikeSend = 
                        btn.querySelector('svg') ||
                        btn.textContent?.includes('发送') ||
                        btn.textContent?.includes('Send') ||
                        btn.getAttribute('aria-label')?.toLowerCase().includes('send') ||
                        btn.className?.toLowerCase().includes('send') ||
                        btn.className?.toLowerCase().includes('submit');
                    
                    if (isVisible && isNotDisabled && looksLikeSend) {
                        sendButton = btn;
                        console.log('[WillChat] Found send button near input');
                        break;
                    }
                }
                if (sendButton) break;
                parent = parent.parentElement;
            }
        }
        
        // 发送消息的函数
        const sendMessage = () => {
            // 通义千问、豆包、DeepSeek 优先使用 Enter 键发送
            if (isQwen || isDoubao || isDeepSeek) {
                console.log('[WillChat] Using Enter key to send (Qwen/Doubao/DeepSeek)...');
                input.focus();
                
                // 模拟 Enter 键按下
                const keydownEvent = new KeyboardEvent('keydown', {
                    key: 'Enter',
                    code: 'Enter',
                    keyCode: 13,
                    which: 13,
                    bubbles: true,
                    cancelable: true,
                    view: window
                });
                
                const keypressEvent = new KeyboardEvent('keypress', {
                    key: 'Enter',
                    code: 'Enter',
                    keyCode: 13,
                    which: 13,
                    bubbles: true,
                    cancelable: true,
                    view: window
                });
                
                const keyupEvent = new KeyboardEvent('keyup', {
                    key: 'Enter',
                    code: 'Enter',
                    keyCode: 13,
                    which: 13,
                    bubbles: true,
                    cancelable: true,
                    view: window
                });
                
                input.dispatchEvent(keydownEvent);
                input.dispatchEvent(keypressEvent);
                input.dispatchEvent(keyupEvent);
                
                // 如果 Enter 没生效，尝试点击按钮
                setTimeout(() => {
                    if (sendButton) {
                        console.log('[WillChat] Enter might not work, trying button click...');
                        sendButton.click();
                    }
                }, 200);
                
                return;
            }
            
            // 其他网站优先使用按钮
            if (sendButton) {
                console.log('[WillChat] Clicking send button...');
                sendButton.click();
            } else {
                // 如果没找到按钮，尝试模拟 Enter 键
                console.log('[WillChat] No send button found, trying Enter key...');
                const enterEvent = new KeyboardEvent('keydown', {
                    key: 'Enter',
                    code: 'Enter',
                    keyCode: 13,
                    which: 13,
                    bubbles: true,
                    cancelable: true
                });
                input.dispatchEvent(enterEvent);
            }
        };
        
        sendMessage();
    }, 500);
})();
`, message)

	panel.ExecJS(js)
	return nil
}

// SendMessageToAllPanels 向所有面板发送消息
func (s *MultiaskService) SendMessageToAllPanels(message string) []string {
	s.panelsLock.RLock()
	ids := make([]string, 0, len(s.panels))
	for id := range s.panels {
		ids = append(ids, id)
	}
	s.panelsLock.RUnlock()

	var errors []string
	for _, id := range ids {
		if err := s.SendMessageToPanel(id, message); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", id, err))
		}
	}
	return errors
}

// GetPanelIDs 获取所有面板 ID
func (s *MultiaskService) GetPanelIDs() []string {
	s.panelsLock.RLock()
	defer s.panelsLock.RUnlock()

	ids := make([]string, 0, len(s.panels))
	for id := range s.panels {
		ids = append(ids, id)
	}
	return ids
}

// FocusPanel 聚焦面板
func (s *MultiaskService) FocusPanel(id string) error {
	s.panelsLock.RLock()
	panel, exists := s.panels[id]
	s.panelsLock.RUnlock()

	if !exists {
		return fmt.Errorf("panel %s not found", id)
	}

	panel.Focus()
	return nil
}
