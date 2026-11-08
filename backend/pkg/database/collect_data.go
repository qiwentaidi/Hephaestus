package database

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
	"hephaestus-backend/pkg/types"
)

// Reuse the existing text column for versioned, structured table data.
type storedCollectData struct {
	Version int             `json:"version"`
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
}

func ValidateCollectData(result types.CollectResult) error {
	if result.Columns == nil || result.Rows == nil || result.RowsCount < 0 {
		return fmt.Errorf("采集结果必须包含 columns、rows 和有效的 rowsCount")
	}
	for _, row := range result.Rows {
		if row == nil || len(row) != len(result.Columns) {
			return fmt.Errorf("采集结果行宽与列数不一致")
		}
	}
	if _, err := json.Marshal(result.Rows); err != nil {
		return fmt.Errorf("采集结果包含无法序列化的值: %w", err)
	}
	return nil
}

func EncodeCollectData(result types.CollectResult) (string, error) {
	if err := ValidateCollectData(result); err != nil {
		return "", err
	}
	data, err := json.Marshal(storedCollectData{Version: 1, Columns: result.Columns, Rows: result.Rows})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func DecodeCollectData(content string, count int) (types.RowData, error) {
	empty := types.RowData{Columns: []string{}, Rows: [][]interface{}{}, RowsCount: count}
	if strings.HasPrefix(strings.TrimSpace(content), "{") {
		var stored storedCollectData
		decoder := json.NewDecoder(strings.NewReader(content))
		decoder.UseNumber()
		if err := decoder.Decode(&stored); err != nil {
			return empty, fmt.Errorf("读取结构化采集数据失败: %w", err)
		}
		if err := decoder.Decode(new(interface{})); err != io.EOF {
			return empty, fmt.Errorf("结构化采集数据包含多余内容")
		}
		if stored.Version != 1 {
			return empty, fmt.Errorf("不支持的采集数据版本: %d", stored.Version)
		}
		if err := ValidateCollectData(types.CollectResult{Columns: stored.Columns, Rows: stored.Rows, RowsCount: count}); err != nil {
			return empty, err
		}
		return types.RowData{Columns: stored.Columns, Rows: stored.Rows, RowsCount: count}, nil
	}
	data, err := legacyCollectData(content, count)
	if err != nil {
		return empty, err
	}
	return data, nil
}

// Parse old tables on the server. Only text reaches the API; attributes and
// active elements never become browser markup or trigger resource requests.
func legacyCollectData(content string, count int) (types.RowData, error) {
	data := types.RowData{Columns: []string{}, Rows: [][]interface{}{}, RowsCount: count}
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return data, fmt.Errorf("解析历史采集结果失败: %w", err)
	}
	var table *html.Node
	var findTable func(*html.Node)
	findTable = func(n *html.Node) {
		if table != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "table" {
			table = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findTable(c)
		}
	}
	findTable(doc)
	if table == nil {
		return data, fmt.Errorf("历史记录缺少表格数据，请重新采集")
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n != table && n.Type == html.ElementNode && n.Data == "table" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "tr" {
			var cells []*html.Node
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "th" || c.Data == "td") {
					cells = append(cells, c)
				}
			}
			if len(cells) == 0 {
				return
			}
			if len(data.Columns) == 0 && cells[0].Data == "th" {
				for _, c := range cells {
					data.Columns = append(data.Columns, legacyCellText(c))
				}
				return
			}
			row := make([]interface{}, len(cells))
			for i, c := range cells {
				row[i] = legacyCellText(c)
				// Only the old renderer's whole-cell NULL marker is nullable.
				// A span embedded in other text must not discard that text.
				child := c.FirstChild
				if child != nil && child == c.LastChild && child.Type == html.ElementNode && child.Data == "span" && legacyCellText(child) == "NULL" {
					for _, attr := range child.Attr {
						if attr.Key == "class" && strings.Contains(" "+attr.Val+" ", " italic ") {
							row[i] = nil
						}
					}
				}
			}
			data.Rows = append(data.Rows, row)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(table)
	if len(data.Columns) == 0 {
		return data, fmt.Errorf("历史记录缺少列名，请重新采集")
	}
	if err := ValidateCollectData(types.CollectResult{Columns: data.Columns, Rows: data.Rows, RowsCount: count}); err != nil {
		return types.RowData{}, err
	}
	return data, nil
}

func legacyCellText(n *html.Node) string {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "iframe", "object", "embed", "template":
			return ""
		case "br":
			return "\n"
		}
	}
	if n.Type == html.TextNode {
		return n.Data
	}
	var text strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text.WriteString(legacyCellText(c))
	}
	return text.String()
}
