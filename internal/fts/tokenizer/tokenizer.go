package tokenizer

import (
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/go-ego/gse"
	"github.com/mozillazg/go-pinyin"
)

const (
	// MaxContentTokens limits the number of tokens for content to prevent oversized FTS index entries
	MaxContentTokens = 10000
	// MaxPinyinChars limits pinyin generation to Chinese text under this character count
	MaxPinyinChars = 200
)

var (
	segOnce sync.Once
	seg     gse.Segmenter
	segMu   sync.Mutex // mutex for concurrent access to segmenter

	pinyinArgs pinyin.Args
)

// initSegmenter initializes the gse segmenter singleton
func initSegmenter() {
	segOnce.Do(func() {
		seg.AlphaNum = true
		seg.SkipLog = true
		_ = seg.LoadDict() // load default dictionary
		pinyinArgs = pinyin.NewArgs()
		pinyinArgs.Style = pinyin.Normal
		// Keep non-Chinese characters as-is
		pinyinArgs.Fallback = func(r rune, a pinyin.Args) []string {
			return []string{string(r)}
		}
	})
}

// TokenizeName tokenizes a file name for FTS indexing
// It removes the extension, segments the name, and generates pinyin tokens for Chinese characters
func TokenizeName(originalName string) string {
	initSegmenter()

	// Remove extension for tokenization
	ext := filepath.Ext(originalName)
	nameWithoutExt := strings.TrimSuffix(originalName, ext)

	// Segment the name
	segMu.Lock()
	tokens := seg.CutSearch(nameWithoutExt, true)
	segMu.Unlock()

	// Clean and dedupe tokens
	tokenSet := make(map[string]struct{})
	var result []string

	for _, token := range tokens {
		token = normalizeToken(token)
		if token == "" {
			continue
		}
		if _, exists := tokenSet[token]; !exists {
			tokenSet[token] = struct{}{}
			result = append(result, token)
		}
	}

	// Generate pinyin tokens for Chinese text (if short enough)
	chineseText := extractChinese(nameWithoutExt)
	if len([]rune(chineseText)) <= MaxPinyinChars && chineseText != "" {
		pinyinTokens := generatePinyinTokens(chineseText)
		for _, pt := range pinyinTokens {
			if _, exists := tokenSet[pt]; !exists {
				tokenSet[pt] = struct{}{}
				result = append(result, pt)
			}
		}
	}

	// Add extension as token (without dot, lowercase)
	if ext != "" {
		extToken := strings.ToLower(strings.TrimPrefix(ext, "."))
		if _, exists := tokenSet[extToken]; !exists {
			result = append(result, extToken)
		}
	}

	return strings.Join(result, " ")
}

// TokenizeContent tokenizes document content for FTS indexing
// It segments the content and applies token limits to prevent oversized index entries
func TokenizeContent(content string) string {
	initSegmenter()

	// Segment content
	segMu.Lock()
	tokens := seg.CutSearch(content, true)
	segMu.Unlock()

	// Clean and dedupe tokens with limit
	tokenSet := make(map[string]struct{})
	var result []string

	for _, token := range tokens {
		if len(result) >= MaxContentTokens {
			break
		}
		token = normalizeToken(token)
		if token == "" {
			continue
		}
		if _, exists := tokenSet[token]; !exists {
			tokenSet[token] = struct{}{}
			result = append(result, token)
		}
	}

	return strings.Join(result, " ")
}

// BuildMatchQuery builds an FTS5 MATCH query string from user input
// It tokenizes the input and generates prefix-match queries joined by AND (implicit in FTS5)
func BuildMatchQuery(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return ""
	}

	initSegmenter()

	// Segment the keyword
	segMu.Lock()
	tokens := seg.CutSearch(keyword, true)
	segMu.Unlock()

	// Build match query with prefix matching
	var queryParts []string
	seen := make(map[string]struct{})

	for _, token := range tokens {
		token = normalizeToken(token)
		if token == "" {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}

		// Escape FTS5 special characters and add prefix match
		escaped := escapeFTS5Token(token)
		queryParts = append(queryParts, escaped+"*")
	}

	// Also try to match pinyin if there's Chinese input
	chineseText := extractChinese(keyword)
	if chineseText != "" && len([]rune(chineseText)) <= MaxPinyinChars {
		pinyinTokens := generatePinyinTokens(chineseText)
		for _, pt := range pinyinTokens {
			if _, exists := seen[pt]; exists {
				continue
			}
			seen[pt] = struct{}{}
			escaped := escapeFTS5Token(pt)
			queryParts = append(queryParts, escaped+"*")
		}
	}

	if len(queryParts) == 0 {
		return ""
	}

	// Join with space (implicit AND in FTS5)
	return strings.Join(queryParts, " ")
}

// normalizeToken cleans a token: lowercase, trim whitespace, skip empty/punctuation-only
func normalizeToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.ToLower(token)

	// Skip empty tokens
	if token == "" {
		return ""
	}

	// Skip tokens that are only whitespace or punctuation
	hasAlphaNum := false
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasAlphaNum = true
			break
		}
	}
	if !hasAlphaNum {
		return ""
	}

	return token
}

// extractChinese extracts Chinese characters from text
func extractChinese(text string) string {
	var sb strings.Builder
	for _, r := range text {
		if isChinese(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// isChinese checks if a rune is a Chinese character
func isChinese(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

// generatePinyinTokens generates pinyin tokens from Chinese text
// Returns: [full pinyin joined, first letter abbreviation]
func generatePinyinTokens(chineseText string) []string {
	if chineseText == "" {
		return nil
	}

	// Get pinyin for each character
	pys := pinyin.LazyPinyin(chineseText, pinyinArgs)
	if len(pys) == 0 {
		return nil
	}

	var result []string

	// Full pinyin joined (e.g., "zhongguoren")
	fullPinyin := strings.Join(pys, "")
	if fullPinyin != "" {
		result = append(result, fullPinyin)
	}

	// First letter abbreviation (e.g., "zgr")
	var abbrev strings.Builder
	for _, py := range pys {
		if len(py) > 0 {
			abbrev.WriteByte(py[0])
		}
	}
	if abbrev.Len() > 0 {
		result = append(result, abbrev.String())
	}

	return result
}

// escapeFTS5Token escapes special characters for FTS5 query
func escapeFTS5Token(token string) string {
	// FTS5 special characters that need to be escaped or removed: " ' * ( ) : ^ -
	// We'll remove them as they're unlikely to be part of meaningful search terms
	var sb strings.Builder
	for _, r := range token {
		switch r {
		case '"', '\'', '*', '(', ')', ':', '^', '-':
			// skip special characters
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
