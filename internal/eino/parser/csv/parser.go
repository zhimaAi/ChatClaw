package csv

import (
	"context"
	"encoding/csv"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
)

// Config CSV 解析器配置
type Config struct {
	// Comma 字段分隔符，默认为 ','
	Comma rune
	// ColumnSeparator 输出文本的列分隔符，默认为 "\t"
	ColumnSeparator string
	// RowSeparator 行分隔符，默认为 "\n"
	RowSeparator string
	// LazyQuotes 是否允许非标准引号
	LazyQuotes bool
}

// Parser CSV 文件解析器
type Parser struct {
	comma           rune
	columnSeparator string
	rowSeparator    string
	lazyQuotes      bool
}

// NewParser 创建新的 CSV 解析器
func NewParser(ctx context.Context, config *Config) (*Parser, error) {
	comma := ','
	colSep := "\t"
	rowSep := "\n"
	lazyQuotes := true

	if config != nil {
		if config.Comma != 0 {
			comma = config.Comma
		}
		if config.ColumnSeparator != "" {
			colSep = config.ColumnSeparator
		}
		if config.RowSeparator != "" {
			rowSep = config.RowSeparator
		}
		lazyQuotes = config.LazyQuotes
	}

	return &Parser{
		comma:           comma,
		columnSeparator: colSep,
		rowSeparator:    rowSep,
		lazyQuotes:      lazyQuotes,
	}, nil
}

// Parse 解析 CSV 文件并返回文档列表
func (p *Parser) Parse(ctx context.Context, reader io.Reader, opts ...parser.Option) ([]*schema.Document, error) {
	// 获取通用选项
	commonOpts := parser.GetCommonOptions(&parser.Options{}, opts...)

	// 创建 CSV 读取器
	csvReader := csv.NewReader(reader)
	csvReader.Comma = p.comma
	csvReader.LazyQuotes = p.lazyQuotes
	csvReader.FieldsPerRecord = -1 // 允许可变数量的字段

	// 读取所有记录
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	// 构建内容
	var content strings.Builder
	for i, record := range records {
		if i > 0 {
			content.WriteString(p.rowSeparator)
		}
		content.WriteString(strings.Join(record, p.columnSeparator))
	}

	// 构建元数据
	metadata := make(map[string]any)
	if commonOpts.URI != "" {
		metadata["_source"] = commonOpts.URI
	}
	metadata["_row_count"] = len(records)
	if len(records) > 0 {
		metadata["_column_count"] = len(records[0])
	}
	for k, v := range commonOpts.ExtraMeta {
		metadata[k] = v
	}

	return []*schema.Document{
		{
			Content:  content.String(),
			MetaData: metadata,
		},
	}, nil
}
