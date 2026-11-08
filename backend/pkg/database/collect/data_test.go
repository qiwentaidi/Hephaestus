package collect

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"hephaestus-backend/pkg/types"
)

func TestStructuredCollectDataRoundTrip(t *testing.T) {
	for _, kind := range []string{"string", "hash", "list", "set", "zset", "stream"} {
		t.Run("redis "+kind, func(t *testing.T) {
			var value interface{} = "dGVzdA== 中文 <img src=x onerror=alert(1)>"
			if kind == "zset" {
				value = 1.25
			}
			if kind == "stream" {
				value = `{"message":"中文","html":"<svg onload=alert(1)>"}`
			}
			result := types.CollectResult{Columns: []string{"key", "type", "field", "value"}, Rows: [][]interface{}{{"key:<script>", kind, "field:&", value}}, RowsCount: 42}
			encoded, err := encodeCollectData(result)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeCollectData(encoded, result.RowsCount)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := json.Marshal(types.RowData{Columns: result.Columns, Rows: result.Rows, RowsCount: result.RowsCount})
			got, _ := json.Marshal(decoded)
			if string(got) != string(want) {
				t.Fatalf("round trip changed data: %s != %s", got, want)
			}
		})
	}
}

func TestStructuredCollectDataSupportsJSONAndEmptyRows(t *testing.T) {
	for _, rows := range [][][]interface{}{{{nil, true, map[string]interface{}{"nested": []interface{}{"<script>", false}}}}, {}} {
		result := types.CollectResult{Columns: []string{"null", "bool", "object"}, Rows: rows, RowsCount: len(rows)}
		encoded, err := encodeCollectData(result)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeCollectData(encoded, result.RowsCount)
		if err != nil || !reflect.DeepEqual(decoded.Rows, rows) {
			t.Fatalf("JSON cells changed: %#v, %v", decoded, err)
		}
	}
}

func TestCollectDataRejectsMalformedStructuredInput(t *testing.T) {
	for _, result := range []types.CollectResult{
		{},
		{Columns: []string{"x"}, Rows: [][]interface{}{{"a", "b"}}},
		{Columns: []string{"x"}, Rows: [][]interface{}{{math.Inf(1)}}},
		{Columns: []string{"x"}, Rows: [][]interface{}{{"a"}}, RowsCount: -1},
		{Columns: []string{}, Rows: [][]interface{}{nil}},
	} {
		if _, err := encodeCollectData(result); err == nil {
			t.Fatalf("accepted invalid input: %#v", result)
		}
	}
	for _, input := range []string{`{"version":1,"columns":["x"],"rows":`, `{"version":2,"columns":[],"rows":[]}`, `{"version":1,"columns":["x"],"rows":[[]]}`, `{"version":1,"columns":[],"rows":[]} {}`, `<img src=x onerror=alert(1)>`} {
		data, err := decodeCollectData(input, 0)
		if err == nil || data.Columns == nil || data.Rows == nil {
			t.Fatalf("unsafe fallback or missing error for %q: %#v, %v", input, data, err)
		}
	}
}

func TestLegacyCollectDataExtractsOnlyText(t *testing.T) {
	input := `<div><table><thead><tr><th>键 &amp; 值</th><th>nullable</th></tr></thead><tbody>` +
		`<tr><td><div title="ignored">&lt;img src=x onerror=alert(1)&gt; 中文</div></td><td><span class="text-muted-foreground italic">NULL</span></td></tr>` +
		`<tr><td><script>bad()</script><svg onload="bad()"><text>safe text</text></svg><img src="https://example.invalid">dGVzdA==</td><td><div>NULL</div></td></tr>` +
		`</tbody></table></div>`
	data, err := decodeCollectData(input, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(data.Columns, []string{"键 & 值", "nullable"}) || data.Rows[0][0] != "<img src=x onerror=alert(1)> 中文" || data.Rows[0][1] != nil || data.Rows[1][0] != "safe textdGVzdA==" || data.Rows[1][1] != "NULL" || data.RowsCount != 20 {
		t.Fatalf("legacy text/NULL distinction lost: %#v", data)
	}
	encoded, err := encodeCollectData(types.CollectResult{Columns: data.Columns, Rows: data.Rows, RowsCount: data.RowsCount})
	if err != nil || strings.Contains(encoded, "<img") {
		t.Fatalf("legacy value was not JSON encoded: %s, %v", encoded, err)
	}
}

func TestNormalizeDatabaseCellPreservesTextBinaryAndLargeIntegers(t *testing.T) {
	cases := []struct{ input, want interface{} }{
		{[]byte("中文 dGVzdA=="), "中文 dGVzdA=="},
		{"dGVzdA==", "dGVzdA=="},
		{[]byte{0xff, 0x00}, map[string]string{"encoding": "base64", "data": "/wA="}},
		{int64(9007199254740993), "9007199254740993"},
		{uint64(18446744073709551615), "18446744073709551615"},
		{1.25, 1.25}, {true, true}, {nil, nil},
	}
	for _, tc := range cases {
		if got := normalizeDatabaseCell(tc.input); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("normalize(%#v) = %#v, want %#v", tc.input, got, tc.want)
		}
	}
}

func TestLegacyNullMarkerDoesNotDiscardSurroundingText(t *testing.T) {
	data, err := decodeCollectData(`<table><tr><th>x</th></tr><tr><td><span class="italic">NULL</span>suffix</td></tr></table>`, 1)
	if err != nil || data.Rows[0][0] != "NULLsuffix" {
		t.Fatalf("discarded non-null text: %+v, %v", data, err)
	}
}
