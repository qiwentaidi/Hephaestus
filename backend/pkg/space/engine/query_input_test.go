package engine

import (
	"context"
	"testing"
)

func TestAggregateQueryNormalization(t *testing.T) {
	for _, tc := range []struct {
		query        string
		kind         QueryType
		engine, want string
	}{
		{`domain="starrides.com.cn"`, QueryTypeDomain, "fofa", `domain="starrides.com.cn"`},
		{`domain="starrides.com.cn"`, QueryTypeDomain, "hunter", `domain.suffix="starrides.com.cn"`},
		{`domain="starrides.com.cn"`, QueryTypeDomain, "quake", `domain:"starrides.com.cn"`},
		{`"耀出行"`, QueryTypeBody, "fofa", `body="耀出行"`},
		{`"耀出行"`, QueryTypeBody, "hunter", `web.body="耀出行"`},
		{`耀出行`, QueryTypeBody, "quake", `body:"耀出行"`},
		{`domain:example.com`, QueryTypeDefault, "hunter", `domain.suffix="example.com"`},
		{`domain="example.com"`, QueryTypeDefault, "quake", `domain:"example.com"`},
		{`"say \"hello\""`, QueryTypeTitle, "fofa", `title="say \"hello\""`},
		{`title:"RESEARCH AND DEVELOPMENT"`, QueryTypeDefault, "fofa", `title="RESEARCH AND DEVELOPMENT"`},
		{`domain:example.com AND port:443`, QueryTypeDefault, "fofa", `domain="example.com" && port="443"`},
	} {
		t.Run(tc.engine+"/"+tc.query, func(t *testing.T) {
			got, err := DefaultTranspiler.TranslateQueryChecked(tc.query, tc.engine, tc.kind)
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

type queryCaptureEngine struct {
	Engine
	name  string
	query string
	calls int
}

func (e *queryCaptureEngine) Name() string { return e.name }
func (e *queryCaptureEngine) Search(req *SearchRequest) *SearchResponse {
	e.calls++
	e.query = req.Query
	return &SearchResponse{}
}

func TestAggregateSearchRejectsInvalidInputBeforeEngineCall(t *testing.T) {
	for _, tc := range []struct {
		query  string
		kind   QueryType
		engine string
	}{
		{`title="example.com"`, QueryTypeDomain, "fofa"},
		{`domain="example.com" AND port=443`, QueryTypeDomain, "fofa"},
		{`"unterminated`, QueryTypeBody, "fofa"},
		{`""`, QueryTypeBody, "fofa"},
		{`example.com`, QueryType("typo"), "fofa"},
		{`example.com`, QueryTypeLink, "censys"},
	} {
		e := &queryCaptureEngine{name: tc.engine}
		a := &Aggregator{transpiler: DefaultTranspiler}
		result := a.searchSingleEngine(context.Background(), e, &AggregatedSearchRequest{Query: tc.query, QueryType: tc.kind})
		if e.calls != 0 || result.Status.Success || result.Status.Message == "" {
			t.Fatalf("invalid query sent to engine: %+v, calls=%d", result, e.calls)
		}
	}
}

func TestAggregateSearchSendsNormalizedQuery(t *testing.T) {
	e := &queryCaptureEngine{name: "fofa"}
	a := &Aggregator{transpiler: DefaultTranspiler}
	result := a.searchSingleEngine(context.Background(), e, &AggregatedSearchRequest{Query: `domain="example.com"`, QueryType: QueryTypeDomain})
	if !result.Status.Success || e.calls != 1 || e.query != `domain="example.com"` {
		t.Fatalf("result=%+v engine=%+v", result, e)
	}
}
