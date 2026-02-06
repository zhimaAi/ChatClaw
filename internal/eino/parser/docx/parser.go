package docx

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
	"github.com/nguyenthenguyen/docx"
)

// Config Docx 解析器配置
type Config struct {
	// ParagraphSeparator 段落分隔符，默认为 "\n\n"
	ParagraphSeparator string
}

// Parser Microsoft Word (.docx) 文件解析器
type Parser struct {
	paragraphSeparator string
}

// NewParser 创建新的 Docx 解析器
func NewParser(ctx context.Context, config *Config) (*Parser, error) {
	separator := "\n\n"
	if config != nil && config.ParagraphSeparator != "" {
		separator = config.ParagraphSeparator
	}
	return &Parser{
		paragraphSeparator: separator,
	}, nil
}

// Parse 解析 docx 文件并返回文档列表
func (p *Parser) Parse(ctx context.Context, reader io.Reader, opts ...parser.Option) ([]*schema.Document, error) {
	// 获取通用选项
	commonOpts := parser.GetCommonOptions(&parser.Options{}, opts...)

	// 将内容读取到临时文件（docx 库需要文件路径）
	tmpFile, err := os.CreateTemp("", "docx-*.docx")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, reader); err != nil {
		return nil, err
	}
	tmpFile.Close()

	// 打开 docx 文件
	r, err := docx.ReadDocxFile(tmpFile.Name())
	if err != nil {
		return nil, err
	}
	defer r.Close()

	// 提取文本内容
	docx1 := r.Editable()
	content := docx1.GetContent()

	// 清理内容 - 从 XML 中提取纯文本
	content = extractPlainText(content)

	// 构建元数据
	metadata := make(map[string]any)
	if commonOpts.URI != "" {
		metadata["_source"] = commonOpts.URI
	}
	for k, v := range commonOpts.ExtraMeta {
		metadata[k] = v
	}

	return []*schema.Document{
		{
			Content:  content,
			MetaData: metadata,
		},
	}, nil
}

// extractPlainText 从 docx XML 内容中提取纯文本
func extractPlainText(xmlContent string) string {
	// 简单提取：移除 XML 标签并规范化空白字符
	var result strings.Builder
	inTag := false
	lastWasSpace := false

	for _, r := range xmlContent {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			// 在某些标签后添加空格
			if !lastWasSpace {
				result.WriteRune(' ')
				lastWasSpace = true
			}
			continue
		}
		if !inTag {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				if !lastWasSpace {
					result.WriteRune(' ')
					lastWasSpace = true
				}
			} else {
				result.WriteRune(r)
				lastWasSpace = false
			}
		}
	}

	// 清理多余的空白
	text := strings.TrimSpace(result.String())
	// 将多个空格替换为单个空格
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return text
}
