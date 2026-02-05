package pdf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
	"github.com/ledongthuc/pdf"
)

// Config PDF 解析器配置
type Config struct {
	// ToPages 是否按页面分割文档，默认为 false
	ToPages bool
	// PageSeparator 页面分隔符，仅在 ToPages=false 时使用，默认为 "\n\n"
	PageSeparator string
}

// Parser PDF 文件解析器
// 使用 ledongthuc/pdf 库进行文本提取
type Parser struct {
	toPages       bool
	pageSeparator string
}

// NewParser 创建新的 PDF 解析器
func NewParser(ctx context.Context, config *Config) (*Parser, error) {
	p := &Parser{
		toPages:       false,
		pageSeparator: "\n\n",
	}
	if config != nil {
		p.toPages = config.ToPages
		if config.PageSeparator != "" {
			p.pageSeparator = config.PageSeparator
		}
	}
	return p, nil
}

// Parse 解析 PDF 文件并返回文档列表
func (p *Parser) Parse(ctx context.Context, reader io.Reader, opts ...parser.Option) ([]*schema.Document, error) {
	// 获取通用选项
	commonOpts := parser.GetCommonOptions(&parser.Options{}, opts...)

	// 将内容读取到临时文件（pdf 库需要文件路径）
	tmpFile, err := os.CreateTemp("", "pdf-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, reader); err != nil {
		return nil, fmt.Errorf("copy to temp file: %w", err)
	}
	tmpFile.Close()

	// 打开 PDF 文件
	f, r, err := pdf.Open(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	// 构建基础元数据
	baseMeta := make(map[string]any)
	if commonOpts.URI != "" {
		baseMeta["_source"] = commonOpts.URI
	}
	for k, v := range commonOpts.ExtraMeta {
		baseMeta[k] = v
	}

	numPages := r.NumPage()
	if numPages == 0 {
		return []*schema.Document{
			{
				Content:  "",
				MetaData: baseMeta,
			},
		}, nil
	}

	// 按页提取文本
	if p.toPages {
		docs := make([]*schema.Document, 0, numPages)
		for pageNum := 1; pageNum <= numPages; pageNum++ {
			page := r.Page(pageNum)
			if page.V.IsNull() {
				continue
			}

			text, err := extractPageText(page)
			if err != nil {
				// 单页解析失败，跳过该页
				continue
			}

			pageMeta := make(map[string]any)
			for k, v := range baseMeta {
				pageMeta[k] = v
			}
			pageMeta["page"] = pageNum
			pageMeta["total_pages"] = numPages

			docs = append(docs, &schema.Document{
				Content:  text,
				MetaData: pageMeta,
			})
		}
		return docs, nil
	}

	// 合并所有页面
	var allText strings.Builder
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := extractPageText(page)
		if err != nil {
			continue
		}

		if allText.Len() > 0 && text != "" {
			allText.WriteString(p.pageSeparator)
		}
		allText.WriteString(text)
	}

	baseMeta["total_pages"] = numPages
	return []*schema.Document{
		{
			Content:  allText.String(),
			MetaData: baseMeta,
		},
	}, nil
}

// extractPageText 从单个页面提取文本
func extractPageText(page pdf.Page) (string, error) {
	var buf bytes.Buffer

	// 使用 GetPlainText 方法提取文本
	// 注意：使用空字符串作为分隔符避免每个字符一行的问题
	texts, err := page.GetPlainText(nil)
	if err != nil {
		return "", err
	}

	buf.WriteString(texts)
	text := buf.String()

	// 清理文本：规范化空白字符
	text = cleanText(text)

	return text, nil
}

// cleanText 清理提取的文本
func cleanText(text string) string {
	// 移除多余的空白行
	lines := strings.Split(text, "\n")
	var result []string
	prevEmpty := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if !prevEmpty {
				result = append(result, "")
				prevEmpty = true
			}
		} else {
			result = append(result, line)
			prevEmpty = false
		}
	}

	return strings.Join(result, "\n")
}
