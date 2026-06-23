package listquery

import (
	"net/url"
	"strings"
	"testing"
)

var spec = Spec{
	"name":  {Col: "p.name", Filter: Text},
	"mgmt":  {Col: "p.mgmt_type", Filter: Enum},
	"start": {Col: "p.plan_start", Filter: Date},
	"cost":  {Col: "p.cost", Filter: Number},
}

func mustValues(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return v
}

func TestSortRejectsUnknownColumn(t *testing.T) {
	c := Build(mustValues(t, "sort="+url.QueryEscape("p.name;DROP TABLE clients--")+"&dir=asc"), spec)
	if c.Order != "" {
		t.Fatalf("unknown sort column leaked into ORDER: %q", c.Order)
	}
}

func TestSortDirOnlyAscDesc(t *testing.T) {
	c := Build(mustValues(t, "sort=name&dir="+url.QueryEscape("asc);DELETE")), spec)
	if !strings.HasSuffix(c.Order, "p.name ASC") {
		t.Fatalf("bad dir not coerced to ASC: %q", c.Order)
	}
}

func TestSortDesc(t *testing.T) {
	c := Build(mustValues(t, "sort=name&dir=desc"), spec)
	if !strings.HasSuffix(c.Order, "p.name DESC") {
		t.Fatalf("dir desc: %q", c.Order)
	}
}

func TestTextFilterIsBound(t *testing.T) {
	c := Build(mustValues(t, "f.name="+url.QueryEscape("x' OR '1'='1")), spec)
	if !strings.Contains(c.Where, "p.name LIKE ?") {
		t.Fatalf("text filter not parameterized: %q", c.Where)
	}
	if c.Args[0] != "%x' OR '1'='1%" {
		t.Fatalf("value not bound verbatim: %#v", c.Args)
	}
}

func TestUnknownFilterKeyIgnored(t *testing.T) {
	c := Build(mustValues(t, "f.evil=1"), spec)
	if c.Where != "" {
		t.Fatalf("unknown filter key produced WHERE: %q", c.Where)
	}
}

func TestEnumInClause(t *testing.T) {
	c := Build(mustValues(t, "f.mgmt=plan,self"), spec)
	if !strings.Contains(c.Where, "p.mgmt_type IN (?,?)") {
		t.Fatalf("enum not IN-parameterized: %q", c.Where)
	}
	// args (order with limit/offset trailing): plan, self, limit, offset
	if c.Args[0] != "plan" || c.Args[1] != "self" {
		t.Fatalf("enum values not bound: %#v", c.Args)
	}
}

func TestDateRange(t *testing.T) {
	c := Build(mustValues(t, "f.start.from=2025-01-01&f.start.to=2025-12-31"), spec)
	if !strings.Contains(c.Where, "p.plan_start >= ?") || !strings.Contains(c.Where, "p.plan_start <= ?") {
		t.Fatalf("date range not built: %q", c.Where)
	}
}

func TestNumberRange(t *testing.T) {
	c := Build(mustValues(t, "f.cost.min=10&f.cost.max=20"), spec)
	if !strings.Contains(c.Where, "p.cost >= ?") || !strings.Contains(c.Where, "p.cost <= ?") {
		t.Fatalf("number range not built: %q", c.Where)
	}
}

func TestLimitClamped(t *testing.T) {
	c := Build(mustValues(t, "limit=99999&page=0"), spec)
	limit := c.Args[len(c.Args)-2]
	offset := c.Args[len(c.Args)-1]
	if limit != MaxLimit || offset != 0 {
		t.Fatalf("limit/offset not clamped: limit=%v offset=%v", limit, offset)
	}
}

func TestPageOffset(t *testing.T) {
	c := Build(mustValues(t, "limit=10&page=3"), spec)
	if c.Args[len(c.Args)-1] != 20 {
		t.Fatalf("offset for page 3 @ limit 10 should be 20, got %v", c.Args[len(c.Args)-1])
	}
}

func TestDefaults(t *testing.T) {
	c := Build(mustValues(t, ""), spec)
	if c.Where != "" || c.Order != "" {
		t.Fatalf("empty query should yield empty where/order: %q %q", c.Where, c.Order)
	}
	if c.Args[len(c.Args)-2] != DefaultLimit || c.Args[len(c.Args)-1] != 0 {
		t.Fatalf("defaults wrong: %#v", c.Args)
	}
}

func TestCountArgsDropsLimitOffset(t *testing.T) {
	c := Build(mustValues(t, "f.name=x"), spec)
	ca := c.CountArgs()
	if len(ca) != 1 || ca[0] != "%x%" {
		t.Fatalf("CountArgs should drop limit/offset: %#v", ca)
	}
}
