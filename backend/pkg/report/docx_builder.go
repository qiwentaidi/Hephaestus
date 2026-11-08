package report

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

type docxReportBuilder struct {
	body strings.Builder
}

func newDocxReportBuilder() *docxReportBuilder {
	return &docxReportBuilder{}
}

func (b *docxReportBuilder) addTitle(text string) {
	b.body.WriteString(docxParagraph("Title", text, false))
}

func (b *docxReportBuilder) addSubtitle(text string) {
	b.body.WriteString(docxParagraph("Subtitle", text, false))
}

func (b *docxReportBuilder) addHeading1(text string) {
	b.body.WriteString(docxParagraph("Heading1", text, false))
}

func (b *docxReportBuilder) addHeading2(text string) {
	b.body.WriteString(docxParagraph("Heading2", text, false))
}

func (b *docxReportBuilder) addParagraph(text string) {
	b.body.WriteString(docxParagraph("", text, false))
}

func (b *docxReportBuilder) addCodeBlock(text string) {
	if strings.TrimSpace(text) == "" {
		text = "-"
	}
	rows := [][]string{{truncateForReportText(text, 12000)}}
	b.addTableWithOptions([]string{"内容"}, rows, true, []bool{true})
}

func (b *docxReportBuilder) addKeyValueTable(rows [][2]string) {
	if len(rows) == 0 {
		return
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		k := strings.TrimSpace(row[0])
		v := strings.TrimSpace(row[1])
		if k == "" {
			k = "-"
		}
		if v == "" {
			v = "-"
		}
		tableRows = append(tableRows, []string{k, v})
	}
	b.addTableWithOptions([]string{"字段", "内容"}, tableRows, true, []bool{false, false})
}

func (b *docxReportBuilder) addTable(headers []string, rows [][]string) {
	b.addTableWithOptions(headers, rows, true, nil)
}

func (b *docxReportBuilder) addTableWithOptions(headers []string, rows [][]string, hasHeader bool, monoCols []bool) {
	cols := len(headers)
	if cols == 0 && len(rows) > 0 {
		cols = len(rows[0])
	}
	if cols == 0 {
		return
	}

	isMonoCol := func(idx int) bool {
		if idx < 0 || idx >= len(monoCols) {
			return false
		}
		return monoCols[idx]
	}

	colW := 9000 / cols
	var sb strings.Builder
	sb.WriteString("<w:tbl>")
	sb.WriteString("<w:tblPr>")
	sb.WriteString("<w:tblW w:w=\"0\" w:type=\"auto\"/>")
	sb.WriteString("<w:tblBorders>")
	sb.WriteString("<w:top w:val=\"single\" w:sz=\"8\" w:space=\"0\" w:color=\"D1D5DB\"/>")
	sb.WriteString("<w:left w:val=\"single\" w:sz=\"8\" w:space=\"0\" w:color=\"D1D5DB\"/>")
	sb.WriteString("<w:bottom w:val=\"single\" w:sz=\"8\" w:space=\"0\" w:color=\"D1D5DB\"/>")
	sb.WriteString("<w:right w:val=\"single\" w:sz=\"8\" w:space=\"0\" w:color=\"D1D5DB\"/>")
	sb.WriteString("<w:insideH w:val=\"single\" w:sz=\"6\" w:space=\"0\" w:color=\"E5E7EB\"/>")
	sb.WriteString("<w:insideV w:val=\"single\" w:sz=\"6\" w:space=\"0\" w:color=\"E5E7EB\"/>")
	sb.WriteString("</w:tblBorders>")
	sb.WriteString("</w:tblPr>")

	sb.WriteString("<w:tblGrid>")
	for i := 0; i < cols; i++ {
		sb.WriteString(fmt.Sprintf("<w:gridCol w:w=\"%d\"/>", colW))
	}
	sb.WriteString("</w:tblGrid>")

	if hasHeader {
		sb.WriteString("<w:tr>")
		for i := 0; i < cols; i++ {
			head := "-"
			if i < len(headers) && strings.TrimSpace(headers[i]) != "" {
				head = headers[i]
			}
			sb.WriteString(docxTableCell(head, colW, true, false))
		}
		sb.WriteString("</w:tr>")
	}

	for _, row := range rows {
		sb.WriteString("<w:tr>")
		for i := 0; i < cols; i++ {
			text := "-"
			if i < len(row) {
				text = row[i]
			}
			sb.WriteString(docxTableCell(text, colW, false, isMonoCol(i)))
		}
		sb.WriteString("</w:tr>")
	}

	sb.WriteString("</w:tbl>")
	b.body.WriteString(sb.String())
	b.body.WriteString(docxParagraph("", "", false))
}

func (b *docxReportBuilder) build(title string) ([]byte, error) {
	bodyXML := b.body.String()
	if strings.TrimSpace(bodyXML) == "" {
		bodyXML = docxParagraph("", "-", false)
	}
	return buildDocxPackage(bodyXML, title)
}

func buildDocxPackage(bodyXML, title string) ([]byte, error) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	documentXML := "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>" +
		"<w:document xmlns:w=\"http://schemas.openxmlformats.org/wordprocessingml/2006/main\" xmlns:r=\"http://schemas.openxmlformats.org/officeDocument/2006/relationships\">" +
		"<w:body>" + bodyXML +
		"<w:sectPr>" +
		"<w:pgSz w:w=\"11906\" w:h=\"16838\"/>" +
		"<w:pgMar w:top=\"1134\" w:right=\"1134\" w:bottom=\"1134\" w:left=\"1134\" w:header=\"708\" w:footer=\"708\" w:gutter=\"0\"/>" +
		"</w:sectPr></w:body></w:document>"

	stylesXML := "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>" +
		"<w:styles xmlns:w=\"http://schemas.openxmlformats.org/wordprocessingml/2006/main\">" +
		"<w:style w:type=\"paragraph\" w:default=\"1\" w:styleId=\"Normal\"><w:name w:val=\"Normal\"/>" +
		"<w:qFormat/><w:rPr><w:rFonts w:ascii=\"Calibri\" w:hAnsi=\"Calibri\" w:eastAsia=\"Microsoft YaHei\"/>" +
		"<w:sz w:val=\"21\"/></w:rPr></w:style>" +
		"<w:style w:type=\"paragraph\" w:styleId=\"Title\"><w:name w:val=\"Title\"/>" +
		"<w:pPr><w:spacing w:before=\"0\" w:after=\"200\"/><w:jc w:val=\"left\"/></w:pPr>" +
		"<w:rPr><w:b/><w:color w:val=\"0F172A\"/><w:rFonts w:ascii=\"Calibri\" w:hAnsi=\"Calibri\" w:eastAsia=\"Microsoft YaHei\"/><w:sz w:val=\"40\"/></w:rPr></w:style>" +
		"<w:style w:type=\"paragraph\" w:styleId=\"Subtitle\"><w:name w:val=\"Subtitle\"/>" +
		"<w:pPr><w:spacing w:before=\"0\" w:after=\"180\"/></w:pPr>" +
		"<w:rPr><w:color w:val=\"475569\"/><w:rFonts w:ascii=\"Calibri\" w:hAnsi=\"Calibri\" w:eastAsia=\"Microsoft YaHei\"/><w:sz w:val=\"20\"/></w:rPr></w:style>" +
		"<w:style w:type=\"paragraph\" w:styleId=\"Heading1\"><w:name w:val=\"Heading 1\"/>" +
		"<w:pPr><w:spacing w:before=\"280\" w:after=\"120\"/></w:pPr>" +
		"<w:rPr><w:b/><w:color w:val=\"1E293B\"/><w:rFonts w:ascii=\"Calibri\" w:hAnsi=\"Calibri\" w:eastAsia=\"Microsoft YaHei\"/><w:sz w:val=\"30\"/></w:rPr></w:style>" +
		"<w:style w:type=\"paragraph\" w:styleId=\"Heading2\"><w:name w:val=\"Heading 2\"/>" +
		"<w:pPr><w:spacing w:before=\"180\" w:after=\"90\"/></w:pPr>" +
		"<w:rPr><w:b/><w:color w:val=\"334155\"/><w:rFonts w:ascii=\"Calibri\" w:hAnsi=\"Calibri\" w:eastAsia=\"Microsoft YaHei\"/><w:sz w:val=\"24\"/></w:rPr></w:style>" +
		"</w:styles>"

	contentTypesXML := "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>" +
		"<Types xmlns=\"http://schemas.openxmlformats.org/package/2006/content-types\">" +
		"<Default Extension=\"rels\" ContentType=\"application/vnd.openxmlformats-package.relationships+xml\"/>" +
		"<Default Extension=\"xml\" ContentType=\"application/xml\"/>" +
		"<Override PartName=\"/word/document.xml\" ContentType=\"application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml\"/>" +
		"<Override PartName=\"/word/styles.xml\" ContentType=\"application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml\"/>" +
		"<Override PartName=\"/docProps/core.xml\" ContentType=\"application/vnd.openxmlformats-package.core-properties+xml\"/>" +
		"<Override PartName=\"/docProps/app.xml\" ContentType=\"application/vnd.openxmlformats-officedocument.extended-properties+xml\"/>" +
		"</Types>"

	rootRelsXML := "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>" +
		"<Relationships xmlns=\"http://schemas.openxmlformats.org/package/2006/relationships\">" +
		"<Relationship Id=\"rId1\" Type=\"http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument\" Target=\"word/document.xml\"/>" +
		"<Relationship Id=\"rId2\" Type=\"http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties\" Target=\"docProps/core.xml\"/>" +
		"<Relationship Id=\"rId3\" Type=\"http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties\" Target=\"docProps/app.xml\"/>" +
		"</Relationships>"

	docRelsXML := "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>" +
		"<Relationships xmlns=\"http://schemas.openxmlformats.org/package/2006/relationships\">" +
		"<Relationship Id=\"rId1\" Type=\"http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles\" Target=\"styles.xml\"/>" +
		"</Relationships>"

	coreXML := "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>" +
		"<cp:coreProperties xmlns:cp=\"http://schemas.openxmlformats.org/package/2006/metadata/core-properties\" " +
		"xmlns:dc=\"http://purl.org/dc/elements/1.1/\" xmlns:dcterms=\"http://purl.org/dc/terms/\" " +
		"xmlns:dcmitype=\"http://purl.org/dc/dcmitype/\" xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\">" +
		"<dc:title>" + docxEscapeText(title) + "</dc:title>" +
		"<dc:creator>Hephaestus</dc:creator>" +
		"<cp:lastModifiedBy>Hephaestus</cp:lastModifiedBy>" +
		"<dcterms:created xsi:type=\"dcterms:W3CDTF\">" + now + "</dcterms:created>" +
		"<dcterms:modified xsi:type=\"dcterms:W3CDTF\">" + now + "</dcterms:modified>" +
		"</cp:coreProperties>"

	appXML := "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>" +
		"<Properties xmlns=\"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties\" " +
		"xmlns:vt=\"http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes\">" +
		"<Application>Hephaestus</Application></Properties>"

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}

	if err := write("[Content_Types].xml", contentTypesXML); err != nil {
		return nil, err
	}
	if err := write("_rels/.rels", rootRelsXML); err != nil {
		return nil, err
	}
	if err := write("docProps/core.xml", coreXML); err != nil {
		return nil, err
	}
	if err := write("docProps/app.xml", appXML); err != nil {
		return nil, err
	}
	if err := write("word/document.xml", documentXML); err != nil {
		return nil, err
	}
	if err := write("word/styles.xml", stylesXML); err != nil {
		return nil, err
	}
	if err := write("word/_rels/document.xml.rels", docRelsXML); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func docxParagraph(styleID, text string, bold bool) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var sb strings.Builder
	sb.WriteString("<w:p>")
	if styleID != "" {
		sb.WriteString("<w:pPr><w:pStyle w:val=\"")
		sb.WriteString(docxEscapeText(styleID))
		sb.WriteString("\"/></w:pPr>")
	}
	sb.WriteString(docxRuns(text, bold, false))
	sb.WriteString("</w:p>")
	return sb.String()
}

func docxRuns(text string, bold bool, mono bool) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		lines = []string{"-"}
	}

	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString("<w:r>")
		if bold || mono {
			sb.WriteString("<w:rPr>")
			if bold {
				sb.WriteString("<w:b/>")
			}
			if mono {
				sb.WriteString("<w:rFonts w:ascii=\"Consolas\" w:hAnsi=\"Consolas\" w:eastAsia=\"Consolas\"/>")
				sb.WriteString("<w:sz w:val=\"18\"/>")
			}
			sb.WriteString("</w:rPr>")
		}
		if line == "" {
			sb.WriteString("<w:t xml:space=\"preserve\"> </w:t>")
		} else {
			sb.WriteString("<w:t xml:space=\"preserve\">")
			sb.WriteString(docxEscapeText(line))
			sb.WriteString("</w:t>")
		}
		sb.WriteString("</w:r>")
		if i < len(lines)-1 {
			sb.WriteString("<w:r><w:br/></w:r>")
		}
	}
	return sb.String()
}

func docxTableCell(text string, width int, header bool, mono bool) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if strings.TrimSpace(text) == "" {
		text = "-"
	}

	var sb strings.Builder
	sb.WriteString("<w:tc><w:tcPr>")
	sb.WriteString(fmt.Sprintf("<w:tcW w:w=\"%d\" w:type=\"dxa\"/>", width))
	if header {
		sb.WriteString("<w:shd w:val=\"clear\" w:color=\"auto\" w:fill=\"E2E8F0\"/>")
	}
	sb.WriteString("</w:tcPr><w:p>")
	sb.WriteString("<w:pPr><w:spacing w:before=\"40\" w:after=\"40\"/></w:pPr>")
	sb.WriteString(docxRuns(text, header, mono))
	sb.WriteString("</w:p></w:tc>")
	return sb.String()
}

func docxEscapeText(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			continue
		}
		switch r {
		case '&':
			out.WriteString("&amp;")
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '"':
			out.WriteString("&quot;")
		case '\'':
			out.WriteString("&apos;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
