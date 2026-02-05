package tokenizer

import (
	"strings"
	"testing"
)

func TestTokenizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string // tokens that should be present
	}{
		{
			name:     "Chinese filename with extension",
			input:    "中国人民.pdf",
			expected: []string{"中国", "人民", "zhongguorenmin", "zgr", "pdf"},
		},
		{
			name:     "English filename",
			input:    "hello_world.docx",
			expected: []string{"hello", "world", "docx"},
		},
		{
			name:     "Mixed Chinese and English",
			input:    "AI人工智能报告.xlsx",
			expected: []string{"ai", "人工智能", "xlsx", "rg", "rengongzhineng"},
		},
		{
			name:     "Numbers and special chars",
			input:    "report-2024-Q1.pdf",
			expected: []string{"report", "2024", "q1", "pdf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TokenizeName(tt.input)
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("TokenizeName(%q) = %q, expected to contain %q", tt.input, result, exp)
				}
			}
		})
	}
}

func TestTokenizeContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Chinese content",
			input:    "今天天气真好，适合出门散步",
			expected: []string{"今天", "天气", "出门", "散步"},
		},
		{
			name:     "English content",
			input:    "The quick brown fox jumps over the lazy dog",
			expected: []string{"quick", "brown", "fox", "jumps"},
		},
		{
			name:     "Mixed content",
			input:    "人工智能AI技术发展迅速",
			expected: []string{"人工智能", "ai", "技术", "发展"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TokenizeContent(tt.input)
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("TokenizeContent(%q) = %q, expected to contain %q", tt.input, result, exp)
				}
			}
		})
	}
}

func TestTokenizeContentLimit(t *testing.T) {
	// Generate a very long content
	var sb strings.Builder
	for i := 0; i < 50000; i++ {
		sb.WriteString("测试 test ")
	}
	content := sb.String()

	result := TokenizeContent(content)
	tokens := strings.Fields(result)

	if len(tokens) > MaxContentTokens {
		t.Errorf("TokenizeContent should limit tokens to %d, got %d", MaxContentTokens, len(tokens))
	}
}

func TestBuildMatchQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string // query parts that should be present
	}{
		{
			name:     "Chinese keyword",
			input:    "中国",
			expected: []string{"中国*", "zhongguo*", "zg*"},
		},
		{
			name:     "English keyword",
			input:    "hello",
			expected: []string{"hello*"},
		},
		{
			name:     "Pinyin input",
			input:    "zgr",
			expected: []string{"zgr*"},
		},
		{
			name:     "Empty input",
			input:    "",
			expected: []string{},
		},
		{
			name:     "Special characters should be filtered",
			input:    "test*query",
			expected: []string{"test*", "query*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildMatchQuery(tt.input)
			if len(tt.expected) == 0 {
				if result != "" {
					t.Errorf("BuildMatchQuery(%q) = %q, expected empty string", tt.input, result)
				}
				return
			}
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("BuildMatchQuery(%q) = %q, expected to contain %q", tt.input, result, exp)
				}
			}
		})
	}
}

func TestEscapeFTS5Token(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"test*query", "testquery"},
		{"hello\"world", "helloworld"},
		{"test:value", "testvalue"},
		{"(test)", "test"},
	}

	for _, tt := range tests {
		result := escapeFTS5Token(tt.input)
		if result != tt.expected {
			t.Errorf("escapeFTS5Token(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractChinese(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"中国人", "中国人"},
		{"Hello世界", "世界"},
		{"Test123", ""},
		{"AI人工智能Report", "人工智能"},
	}

	for _, tt := range tests {
		result := extractChinese(tt.input)
		if result != tt.expected {
			t.Errorf("extractChinese(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestGeneratePinyinTokens(t *testing.T) {
	tests := []struct {
		input       string
		expectedPy  string // full pinyin
		expectedAbb string // abbreviation
	}{
		{"中国", "zhongguo", "zg"},
		{"人民", "renmin", "rm"},
		{"中国人民", "zhongguorenmin", "zgrm"},
	}

	for _, tt := range tests {
		tokens := generatePinyinTokens(tt.input)
		if len(tokens) < 2 {
			t.Errorf("generatePinyinTokens(%q) returned %d tokens, expected at least 2", tt.input, len(tokens))
			continue
		}
		if tokens[0] != tt.expectedPy {
			t.Errorf("generatePinyinTokens(%q) full pinyin = %q, expected %q", tt.input, tokens[0], tt.expectedPy)
		}
		if tokens[1] != tt.expectedAbb {
			t.Errorf("generatePinyinTokens(%q) abbreviation = %q, expected %q", tt.input, tokens[1], tt.expectedAbb)
		}
	}
}

func TestNormalizeToken(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello", "hello"},
		{"  test  ", "test"},
		{"...", ""},       // only punctuation
		{"   ", ""},       // only whitespace
		{"Test123", "test123"},
	}

	for _, tt := range tests {
		result := normalizeToken(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeToken(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}
