package xlsx

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
	"github.com/xuri/excelize/v2"
)

// Config Xlsx 解析器配置
type Config struct {
	// ToSheets 是否将每个工作表拆分为单独的文档
	ToSheets bool
	// ColumnSeparator 列分隔符，默认为 "\t"
	ColumnSeparator string
	// RowSeparator 行分隔符，默认为 "\n"
	RowSeparator string
}

// Parser Microsoft Excel (.xlsx) 文件解析器
type Parser struct {
	toSheets        bool
	columnSeparator string
	rowSeparator    string
}

// NewParser 创建新的 Xlsx 解析器
func NewParser(ctx context.Context, config *Config) (*Parser, error) {
	colSep := "\t"
	rowSep := "\n"
	toSheets := false

	if config != nil {
		if config.ColumnSeparator != "" {
			colSep = config.ColumnSeparator
		}
		if config.RowSeparator != "" {
			rowSep = config.RowSeparator
		}
		toSheets = config.ToSheets
	}

	return &Parser{
		toSheets:        toSheets,
		columnSeparator: colSep,
		rowSeparator:    rowSep,
	}, nil
}

// Parse 解析 xlsx 文件并返回文档列表
func (p *Parser) Parse(ctx context.Context, reader io.Reader, opts ...parser.Option) ([]*schema.Document, error) {
	// 获取通用选项
	commonOpts := parser.GetCommonOptions(&parser.Options{}, opts...)

	// 从 reader 打开 xlsx 文件
	f, err := excelize.OpenReader(reader)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// 获取所有工作表名称
	sheetList := f.GetSheetList()
	if len(sheetList) == 0 {
		return []*schema.Document{}, nil
	}

	// 构建基础元数据
	baseMetadata := make(map[string]any)
	if commonOpts.URI != "" {
		baseMetadata["_source"] = commonOpts.URI
	}
	for k, v := range commonOpts.ExtraMeta {
		baseMetadata[k] = v
	}

	if p.toSheets {
		// 每个工作表返回一个文档
		docs := make([]*schema.Document, 0, len(sheetList))
		for i, sheetName := range sheetList {
			content, err := p.extractSheetContent(f, sheetName)
			if err != nil {
				return nil, err
			}

			// 复制元数据并添加工作表信息
			metadata := make(map[string]any)
			for k, v := range baseMetadata {
				metadata[k] = v
			}
			metadata["_sheet_name"] = sheetName
			metadata["_sheet_index"] = i

			docs = append(docs, &schema.Document{
				Content:  content,
				MetaData: metadata,
			})
		}
		return docs, nil
	}

	// 将所有工作表合并为一个文档
	var allContent strings.Builder
	for i, sheetName := range sheetList {
		if i > 0 {
			allContent.WriteString("\n\n")
		}
		allContent.WriteString(fmt.Sprintf("=== 工作表: %s ===\n", sheetName))
		content, err := p.extractSheetContent(f, sheetName)
		if err != nil {
			return nil, err
		}
		allContent.WriteString(content)
	}

	return []*schema.Document{
		{
			Content:  allContent.String(),
			MetaData: baseMetadata,
		},
	}, nil
}

// extractSheetContent 从单个工作表中提取内容
func (p *Parser) extractSheetContent(f *excelize.File, sheetName string) (string, error) {
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return "", err
	}

	var content strings.Builder
	for i, row := range rows {
		if i > 0 {
			content.WriteString(p.rowSeparator)
		}
		content.WriteString(strings.Join(row, p.columnSeparator))
	}

	return content.String(), nil
}
