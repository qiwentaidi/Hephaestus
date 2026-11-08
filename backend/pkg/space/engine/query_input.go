package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var queryFieldPrefix = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_.]*)\s*[:=]\s*(.*)$`)
var queryBoolean = regexp.MustCompile(`(?i)\s+(AND|OR|NOT)\s+|&&|\|\|`)

// NormalizeQueryInput separates a semantic value from an expression before
// engine templates add their own field name and quotes.
func NormalizeQueryInput(query string, kind QueryType) (string, QueryType, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", kind, fmt.Errorf("query 不能为空")
	}
	if kind == "" {
		kind = QueryTypeDefault
	}
	if kind != QueryTypeDefault {
		if _, ok := DefaultTranspiler.syntaxMap[kind]; !ok {
			return "", kind, fmt.Errorf("unsupported queryType %q", kind)
		}
	}
	if match := queryFieldPrefix.FindStringSubmatch(query); match != nil {
		fieldKind := QueryTypeFromString(match[1])
		value, err := singleQueryValue(match[2])
		if kind == QueryTypeDefault && (err != nil || fieldKind == QueryTypeDefault) {
			// Compound queries retain the aggregate search's unified syntax.
			return query, kind, nil
		}
		if fieldKind == QueryTypeDefault || (kind != QueryTypeDefault && fieldKind != kind) {
			return "", kind, fmt.Errorf("query field %q conflicts with queryType %q; use a plain value or queryType=default with unified syntax", match[1], kind)
		}
		if err != nil {
			return "", kind, err
		}
		return value, fieldKind, nil
	}
	if kind == QueryTypeDefault {
		return query, kind, nil
	}
	value, err := singleQueryValue(query)
	return value, kind, err
}

func singleQueryValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("invalid quoted query value; compound expressions require queryType=default and unified syntax")
		}
		value = decoded
	} else if queryBoolean.MatchString(value) || strings.ContainsAny(value, "\"\r\n") {
		return "", fmt.Errorf("queryType requires a single value; use queryType=default for compound expressions")
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("query value cannot be empty")
	}
	return value, nil
}

// TranslateQueryChecked is shared by aggregate callers, including the AI tools.
// Invalid typed expressions must never be sent to quota-consuming engines.
func (t *Transpiler) TranslateQueryChecked(query, engine string, kind QueryType) (string, error) {
	value, kind, err := NormalizeQueryInput(query, kind)
	if err != nil {
		return "", err
	}
	if kind == QueryTypeDefault {
		return t.translateComplexQuery(value, engine), nil
	}
	if engine == "shodan" {
		if query, ok := shodanQueryForType(value, kind); ok {
			return query, nil
		}
		return "", fmt.Errorf("shodan does not support queryType %s in aggregate search", kind)
	}
	rule := t.syntaxMap[kind]
	formats := map[string]string{"fofa": rule.Fofa, "hunter": rule.Hunter, "quake": rule.Quake, "censys": rule.Censys, "zoomeye": rule.Zoomeye}
	if formats[engine] == "" {
		return "", fmt.Errorf("%s does not support queryType %s in aggregate search", engine, kind)
	}
	// Values are literals, not executable fragments of engine query syntax.
	quoted := strconv.Quote(value)
	return t.formatSimpleQuery(quoted[1:len(quoted)-1], engine, kind), nil
}

// shodanQueryForType maps a typed aggregate value to Shodan's filter syntax.
func shodanQueryForType(value string, kind QueryType) (string, bool) {
	quoted := func(field string) string {
		return fmt.Sprintf(`%s:"%s"`, field, value)
	}
	switch kind {
	case QueryTypeIP:
		return "ip:" + value, true
	case QueryTypePort:
		return "port:" + value, true
	case QueryTypeDomain, QueryTypeHost:
		return quoted("hostname"), true
	case QueryTypeTitle:
		return quoted("http.title"), true
	case QueryTypeHeader, QueryTypeServer:
		return quoted("http.headers"), true
	case QueryTypeBody:
		return quoted("http.html"), true
	case QueryTypeProduct, QueryTypeApp:
		return quoted("product"), true
	case QueryTypeOS:
		return quoted("os"), true
	case QueryTypeCountry:
		return quoted("country"), true
	case QueryTypeCity:
		return quoted("city"), true
	case QueryTypeOrg:
		return quoted("org"), true
	case QueryTypeISP:
		return quoted("isp"), true
	case QueryTypeASN:
		return "asn:" + value, true
	case QueryTypeCert:
		return quoted("ssl"), true
	case QueryTypeIconHash:
		return "http.favicon.hash:" + value, true
	case QueryTypeJarm:
		return "ssl.jarm:" + value, true
	}
	return "", false
}
