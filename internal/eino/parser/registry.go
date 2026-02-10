package parser

import (
	"context"

	"github.com/cloudwego/eino-ext/components/document/parser/html"
	"github.com/cloudwego/eino/components/document/parser"

	csvparser "willclaw/internal/eino/parser/csv"
	docxparser "willclaw/internal/eino/parser/docx"
	pdfparser "willclaw/internal/eino/parser/pdf"
	xlsxparser "willclaw/internal/eino/parser/xlsx"
)

// NewDocumentParser 创建一个支持多种文件格式的文档解析器
// 使用 ExtParser 根据文件扩展名自动选择合适的解析器
func NewDocumentParser(ctx context.Context) (parser.Parser, error) {
	// 创建文本解析器（用于 txt, md 文件）
	textParser := parser.TextParser{}

	// 创建 HTML 解析器
	htmlParser, err := html.NewParser(ctx, &html.Config{})
	if err != nil {
		return nil, err
	}

	// 创建 PDF 解析器（使用自定义解析器，支持中文）
	pdfParser, err := pdfparser.NewParser(ctx, &pdfparser.Config{
		ToPages: false,
	})
	if err != nil {
		return nil, err
	}

	// 创建自定义解析器
	docxParser, err := docxparser.NewParser(ctx, &docxparser.Config{})
	if err != nil {
		return nil, err
	}

	xlsxParser, err := xlsxparser.NewParser(ctx, &xlsxparser.Config{
		ToSheets: false,
	})
	if err != nil {
		return nil, err
	}

	csvParser, err := csvparser.NewParser(ctx, &csvparser.Config{})
	if err != nil {
		return nil, err
	}

	// 创建 ExtParser，注册所有解析器
	extParser, err := parser.NewExtParser(ctx, &parser.ExtParserConfig{
		Parsers: map[string]parser.Parser{
			// PDF 文件
			".pdf": pdfParser,
			// HTML 文件
			".html": htmlParser,
			".htm":  htmlParser,
			// Microsoft Office 文件
			".docx": docxParser,
			".xlsx": xlsxParser,
			// 文本文件
			".txt": textParser,
			".md":  textParser,
			// CSV 文件
			".csv": csvParser,
		},
		FallbackParser: textParser,
	})
	if err != nil {
		return nil, err
	}

	return extParser, nil
}
