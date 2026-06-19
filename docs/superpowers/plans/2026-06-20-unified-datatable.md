# Unified DataTable Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hand-rolled list tables with one generic Excel-like `DataTable.svelte` backed by a safe server-side filter/sort/paginate query engine.

**Architecture:** A new platform package `internal/listquery` builds parameterized WHERE/ORDER/LIMIT clauses from a per-slice column allowlist (the SQL-injection safeguard) and appends them to a constant base SELECT. Each list endpoint gains `{rows,total}` query support. A single `DataTable.svelte` (columns + actions config) drives search/sort/filter/selection/pagination and a Notion-style edit drawer, querying through `createCollectionStore.query()`.

**Tech Stack:** Go 1.26 (database/sql, modernc sqlite), chi; SvelteKit + Svelte 5 runes, Tailwind 4, `@lucide/svelte`. Spec: `docs/superpowers/specs/2026-06-20-unified-datatable-design.md`.

---

## Phase 1 — `internal/listquery` (security-critical core, TDD)

The whole package is pure functions over `url.Values` + a `Spec`. No DB. Test it hard.

### Task 1: Types

**Files:**
- Create: `internal/listquery/listquery.go`

- [ ] **Step 1: Define the spec + result types**

```go
// Package listquery builds safe, parameterized SQL fragments (WHERE/ORDER/LIMIT)
// for list endpoints from a per-resource column allowlist. Client requests never
// supply SQL identifiers or operators — only allowlisted column KEYS and bound
// VALUES — which is what makes dynamic list SQL injection-safe here.
package listquery

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// FilterType selects the operator and value parsing for a column.
type FilterType int

const (
	None   FilterType = iota // sortable but not filterable
	Text                     // LIKE %v%
	Enum                     // IN (?,?,…) from a comma-separated value
	Date                     // >= from AND <= to (f.<key>.from / .to)
	Number                   // >= min AND <= max (f.<key>.min / .max)
)

// ColSpec maps an API key to a CONSTANT SQL column expression we authored.
type ColSpec struct {
	Col    string // e.g. "p.name" — author-controlled constant, never client input
	Filter FilterType
}

// Spec is the allowlist: API key -> ColSpec. Only keys present here are
// filterable/sortable; anything else is rejected.
type Spec map[string]ColSpec

// Defaults bounds pagination.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Clause is the assembled, parameterized SQL tail plus its bound args.
type Clause struct {
	Where string // "" or "AND (...) AND (...)" — caller appends after its own tenant filter
	Order string // " ORDER BY <col> ASC|DESC"
	Limit string // " LIMIT ? OFFSET ?"
	Args  []any  // WHERE args then LIMIT args, in clause order
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/listquery/listquery.go
git commit -m "feat(listquery): spec + clause types"
```

### Task 2: Build the clause (the safeguards live here)

**Files:**
- Modify: `internal/listquery/listquery.go`
- Test: `internal/listquery/listquery_test.go`

- [ ] **Step 1: Write failing tests (hostile inputs first)**

```go
package listquery

import (
	"net/url"
	"strings"
	"testing"
)

var spec = Spec{
	"name": {Col: "p.name", Filter: Text},
	"mgmt": {Col: "p.mgmt_type", Filter: Enum},
	"start": {Col: "p.plan_start", Filter: Date},
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
	c := Build(mustValues(t, "sort=p.name;DROP TABLE participants--&dir=asc"), spec)
	if strings.Contains(c.Order, "DROP") || strings.Contains(c.Order, ";") {
		t.Fatalf("unknown sort column leaked into ORDER: %q", c.Order)
	}
}

func TestSortDirOnlyAscDesc(t *testing.T) {
	c := Build(mustValues(t, "sort=name&dir=asc);DELETE"), spec)
	if !strings.HasSuffix(c.Order, "ASC") {
		t.Fatalf("bad dir not coerced to ASC: %q", c.Order)
	}
}

func TestTextFilterIsBound(t *testing.T) {
	c := Build(mustValues(t, "f.name="+url.QueryEscape("x' OR '1'='1")), spec)
	if !strings.Contains(c.Where, "p.name LIKE ?") {
		t.Fatalf("text filter not parameterized: %q", c.Where)
	}
	if len(c.Args) == 0 || c.Args[0] != "%x' OR '1'='1%" {
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
}

func TestLimitClamped(t *testing.T) {
	c := Build(mustValues(t, "limit=99999&page=0"), spec)
	// page<1 => offset 0; limit clamped to MaxLimit
	if c.Args[len(c.Args)-2] != MaxLimit || c.Args[len(c.Args)-1] != 0 {
		t.Fatalf("limit/offset not clamped: %#v", c.Args[len(c.Args)-2:])
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/listquery/` → undefined: Build

- [ ] **Step 3: Implement `Build`**

```go
// Build assembles a safe Clause from request params using spec as the allowlist.
// Invariants: identifiers come only from spec.Col (constants); every value is a
// bound ? arg; operators are fixed per FilterType; dir ∈ {ASC,DESC}; limit/offset
// are clamped ints. Unknown keys are ignored, never interpolated.
func Build(q url.Values, spec Spec) Clause {
	if spec == nil { // assertion: a nil spec is a programmer error
		panic("listquery.Build: nil spec")
	}
	var where []string
	var args []any

	// Filters. Iterate the SPEC (controlled), not the query, so unknown keys
	// can never reach the SQL.
	for key, col := range spec {
		switch col.Filter {
		case Text:
			if v := q.Get("f." + key); v != "" {
				where = append(where, col.Col+" LIKE ?")
				args = append(args, "%"+v+"%")
			}
		case Enum:
			if v := q.Get("f." + key); v != "" {
				parts := strings.Split(v, ",")
				ph := make([]string, 0, len(parts))
				for _, p := range parts { // bounded by len(parts)
					ph = append(ph, "?")
					args = append(args, p)
				}
				where = append(where, col.Col+" IN ("+strings.Join(ph, ",")+")")
			}
		case Date, Number:
			lo, hi := "from", "to"
			if col.Filter == Number {
				lo, hi = "min", "max"
			}
			if v := q.Get("f." + key + "." + lo); v != "" {
				where = append(where, col.Col+" >= ?")
				args = append(args, v)
			}
			if v := q.Get("f." + key + "." + hi); v != "" {
				where = append(where, col.Col+" <= ?")
				args = append(args, v)
			}
		}
	}

	// Order: sort key must be in spec; dir is asc/desc only.
	order := ""
	if sk := q.Get("sort"); sk != "" {
		if col, ok := spec[sk]; ok {
			dir := "ASC"
			if strings.EqualFold(q.Get("dir"), "desc") {
				dir = "DESC"
			}
			order = " ORDER BY " + col.Col + " " + dir
		}
	}

	// Limit/offset: clamped ints.
	limit := DefaultLimit
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		limit = n
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	offset := 0
	if p, err := strconv.Atoi(q.Get("page")); err == nil && p > 1 {
		offset = (p - 1) * limit
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " AND " + strings.Join(where, " AND ")
	}
	args = append(args, limit, offset)
	return Clause{
		Where: whereSQL,
		Order: order,
		Limit: " LIMIT ? OFFSET ?",
		Args:  args,
	}
}
```

Note: `Where` begins with ` AND ` so callers splice it after their mandatory
`WHERE p.tenant_id = ?`. `fmt` import may be unused — drop it if so (`gofmt`).

- [ ] **Step 4: Run, expect PASS** — `go test ./internal/listquery/`

- [ ] **Step 5: `gofmt -l internal/listquery && go vet ./internal/listquery/`** (must be clean)

- [ ] **Step 6: Commit**

```bash
git add internal/listquery/
git commit -m "feat(listquery): safe clause builder with injection safeguards + tests"
```

> **Determinism note for the planner/executor:** Go map iteration order is random, so multiple WHERE fragments may be emitted in any order. Tests assert on `strings.Contains`, not full-string equality — keep it that way. The args slice order matches emission order; the SQL placeholders and args stay consistent because both are appended in the same loop iteration.

---

## Phase 2 — Participants backend uses listquery (reference)

### Task 3: Repo `Query` + scan

**Files:**
- Modify: `internal/participant/repository.go`
- Modify: `internal/db/queries/participants.sql` is NOT changed; instead add the base SELECT body as a constant in the repo (mirrors `ListParticipants`).

- [ ] **Step 1: Add the spec + base SELECT constant + Query method**

```go
// participantListSelect mirrors the ListParticipants sqlc query body up to the
// WHERE. Keep in sync with internal/db/queries/participants.sql.
const participantListSelect = `SELECT p.*, pm.name AS plan_manager_name
FROM participants p
LEFT JOIN plan_managers pm ON p.plan_manager_id = pm.id AND pm.tenant_id = p.tenant_id
WHERE p.tenant_id = ?`

// ParticipantCols is the listquery allowlist for participants.
var ParticipantCols = listquery.Spec{
	"name":   {Col: "p.name", Filter: listquery.Text},
	"ndis":   {Col: "p.ndis_number", Filter: listquery.Text},
	"email":  {Col: "p.email", Filter: listquery.Text},
	"mgmt":   {Col: "p.mgmt_type", Filter: listquery.Enum},
	"start":  {Col: "p.plan_start", Filter: listquery.Date},
	"pm":     {Col: "pm.name", Filter: listquery.Text},
}

// Query returns one page of participants plus the total row count for the filter.
func (r *ParticipantsRepo) Query(ctx context.Context, tenantID int64, c listquery.Clause) ([]*Participant, int64, error) {
	if tenantID == 0 {
		return nil, 0, errors.New("query participants: tenant id required")
	}
	// total
	var total int64
	countSQL := "SELECT count(*) FROM (" + participantListSelect + c.Where + ")"
	countArgs := append([]any{tenantID}, c.Args[:len(c.Args)-2]...) // drop limit/offset
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count participants: %w", err)
	}
	// page
	sqlText := participantListSelect + c.Where + c.Order + c.Limit
	pageArgs := append([]any{tenantID}, c.Args...)
	rows, err := r.db.QueryContext(ctx, sqlText, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query participants: %w", err)
	}
	defer rows.Close()
	out := make([]*Participant, 0, 50)
	for rows.Next() {
		var f participantFields
		// participants.* column order: id,uuid,tenant_id,name,ndis_number,plan_start,
		// plan_end,mgmt_type,plan_manager_id,email,phone,address,metadata,created_at,
		// updated_at, then plan_manager_name. Verify against the migration before running.
		var tenant int64
		if err := rows.Scan(&f.id, &f.uuid, &tenant, &f.name, &f.ndisNumber, &f.planStart,
			&f.planEnd, &f.mgmtType, &f.planManagerID, &f.email, &f.phone, &f.address,
			&f.metadata, &f.createdAt, &f.updatedAt, &f.planManagerName); err != nil {
			return nil, 0, fmt.Errorf("scan participant: %w", err)
		}
		out = append(out, mapParticipantFields(f))
	}
	return out, total, rows.Err()
}
```

Add the import `"github.com/dknathalage/tallyo/internal/listquery"`.

> **CRITICAL for executor:** the `rows.Scan` column order MUST match
> `participants` table column order from `internal/db/migrations/*.sql` plus the
> trailing `plan_manager_name`. Open the migration and confirm before running.
> If `participantFields` has `mgmtType string` but the column is nullable in
> scan, adjust types to match the existing struct.

- [ ] **Step 2: `go build ./internal/participant/`** — fix scan/type mismatches.

- [ ] **Step 3: Commit**

```bash
git add internal/participant/repository.go
git commit -m "feat(participant): listquery-backed Query (page + total)"
```

### Task 4: Service + handler return `{rows,total}`

**Files:**
- Modify: `internal/participant/service.go`, `internal/participant/handler.go`

- [ ] **Step 1: Service.Query**

```go
// QueryResult is one page plus the unfiltered-by-page total.
type QueryResult struct {
	Rows  []*Participant `json:"rows"`
	Total int64          `json:"total"`
}

// Query returns a page of participants for the given listquery clause.
func (s *Service) Query(ctx context.Context, c listquery.Clause) (QueryResult, error) {
	tenantID := reqctx.MustTenant(ctx)
	rows, total, err := s.repo.Query(ctx, tenantID, c)
	if err != nil {
		return QueryResult{}, err
	}
	if rows == nil {
		rows = []*Participant{} // never null in JSON
	}
	return QueryResult{Rows: rows, Total: total}, nil
}
```

- [ ] **Step 2: Handler.List — detect query params, branch**

Replace the body of `List` so that when any of `sort`,`limit`,`page`, or a
`f.`-prefixed param is present it returns the paged result; otherwise keep the
old `?search=` behavior for back-compat (the old list pages still call it during
migration).

```go
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if isListQuery(q) {
		c := listquery.Build(q, ParticipantCols)
		res, err := h.svc.Query(r.Context(), c)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
		return
	}
	participants, err := h.svc.List(r.Context(), q.Get("search"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, participants)
}

// isListQuery is true when the request carries DataTable query params.
func isListQuery(q url.Values) bool {
	if q.Has("sort") || q.Has("page") || q.Has("limit") {
		return true
	}
	for k := range q {
		if strings.HasPrefix(k, "f.") {
			return true
		}
	}
	return false
}
```

Add imports `net/url`, `strings`, and the listquery import.

- [ ] **Step 3: `go build ./... && go vet ./internal/participant/ && gofmt -l internal/participant`**

- [ ] **Step 4: Manual smoke** — `go run ./cmd/tallyo` then
  `curl -s 'localhost:8080/api/participants?sort=name&dir=desc&limit=2&f.mgmt=plan' -b cookies` → `{rows:[...],total:N}`. (Needs a session cookie; or add a quick Go test.)

- [ ] **Step 5: Commit**

```bash
git add internal/participant/
git commit -m "feat(participant): {rows,total} list query endpoint (back-compat ?search)"
```

---

## Phase 3 — Frontend query plumbing

### Task 5: `crud.query` + store `query()` + types

**Files:**
- Modify: `web/src/lib/api/crud.ts`, `web/src/lib/stores/collection.svelte.ts`
- Modify: `web/src/lib/api/types.ts` (add `ListParams`, `ListResult<T>`)

- [ ] **Step 1: Types**

```ts
// web/src/lib/api/types.ts
export interface ListParams {
	sort?: string;
	dir?: 'asc' | 'desc';
	page?: number;
	limit?: number;
	filters?: Record<string, string>; // key -> encoded value (e.g. "plan,self", or "x" for contains)
}
export interface ListResult<T> {
	rows: T[];
	total: number;
}
```

- [ ] **Step 2: crud.query**

```ts
// crud.ts — add to Crud<T,TInput> interface: query(params: ListParams): Promise<ListResult<T>>;
function toQueryString(p: ListParams): string {
	const u = new URLSearchParams();
	if (p.sort) u.set('sort', p.sort);
	if (p.dir) u.set('dir', p.dir);
	if (p.page) u.set('page', String(p.page));
	if (p.limit) u.set('limit', String(p.limit));
	for (const [k, v] of Object.entries(p.filters ?? {})) {
		if (v !== '') u.set('f.' + k, v);
	}
	const s = u.toString();
	return s ? '?' + s : '';
}
// in createCrud return object:
query: async (params) =>
	(await apiGet<ListResult<T>>(`${base}${toQueryString(params)}`)) ?? { rows: [], total: 0 },
```

- [ ] **Step 3: store query()** — add a `query(params)` method to
  `createCollectionStore` that calls `crud.query`, stores `rows`/`total`/`params`
  as runes, and re-runs the last query on SSE invalidation (replace the `load()`
  call in `onEntity` with: if a query has run, re-run it; else `load()`).

- [ ] **Step 4: `cd web && npm run check`** (0 errors / 0 warnings)

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/api web/src/lib/stores
git commit -m "feat(web): server-side list query in crud + collection store"
```

---

## Phase 4 — `DataTable.svelte`

### Task 6: Build the component

**Files:**
- Create: `web/src/lib/components/DataTable.svelte`
- Reference (port the interaction/markup): `.superpowers/brainstorm/38129-1781908519/datatable-interactive.html`

Build a generic component. Props (Svelte 5 `$props`):

```ts
type Props<T> = {
	title: string;
	columns: Column<T>[];
	store: { rows: T[]; total: number; loading: boolean; query(p: ListParams): Promise<void> };
	rowActions?: RowAction<T>[];   // bulk:true ones show in the selection bar
	onNew?: () => void;
	onRowSave?: (row: T) => Promise<void>; // drawer autosave; omit to disable edit
	detailHref?: (row: T) => string | null; // "Open full page" link
	pageSize?: number;             // default 50
};
```

Behavior to port from the approved prototype:
- **Strip**: title (left) + active-filter chips + `Clear filters`; right = `New`
  button OR selection actions (count + bulk actions w/ lucide icons, danger red,
  `✕` clear). Selection styled dark-text-on-light (matches strip).
- **Header menu** (click anywhere on a header): Asc/Desc side-by-side buttons (if
  `sortable`), then filter control by `filter` type (text=contains input,
  enum=checkbox list of `values`, date=from/to, number=min/max). Live, no Apply,
  no Clear-in-menu. Header shows ▲/▼ when active sort, blue funnel when filtered.
- **Selection**: row checkbox + select-all (current page), Shift+click range,
  ⌘/Ctrl+A select-all-visible (ignore while typing), click-outside clears, Esc
  staged (drawer→menu→selection).
- **Drawer**: right slide-in; fields from columns; debounced `onRowSave`; "✓ saved";
  `detailHref` → "Open full page ↗".
- **Query wiring**: any state change (sort/dir/filters/page) → debounce ~200ms →
  `store.query({sort,dir,page,limit,filters})`. Map enum Set → comma string;
  text → raw contains; date/number → encode as `key.from`/`key.to` etc. **Decision
  for executor:** pass range filters as separate `filters['start.from']` entries
  (the crud `toQueryString` prefixes `f.`), so the server sees `f.start.from`.
- **SSE/drawer guard (spec risk):** while the drawer is open and dirty, a store
  re-query updates the table behind it but must NOT reset the open drawer's field
  state. Keep drawer field state local to the drawer; only re-hydrate it when the
  user opens a (different) row.

- [ ] **Step 1: Implement the component.** Use Tailwind classes matching the app
  (see `participants/+page.svelte` for the existing palette: `rounded border
  border-gray-200 bg-white`, `bg-gray-50` headers, `text-sm`).
- [ ] **Step 2: `cd web && npm run check`** — 0 errors / 0 warnings. Fix all
  `any`/`@ts-ignore` per coding rule 10 (inline-comment justify if unavoidable).
- [ ] **Step 3: Commit** — `feat(web): generic DataTable component`

---

## Phase 5 — Migrate the Participants page (reference)

### Task 7: Rewrite `participants/+page.svelte` on DataTable

**Files:**
- Modify: `web/src/routes/participants/+page.svelte`

- [ ] **Step 1:** Replace the hand-rolled table + search box with `<DataTable>`,
  declaring columns (name/ndis/window→start date/mgmt enum/pm), `rowActions`
  (Delete via existing `bulk-delete`; Export later), `onNew` (keep the existing
  create Modal), `onRowSave` → `participants.crud.update`, `detailHref` →
  `/participants/{id}`. Switch the store call from `load()` to
  `query({page:1, limit:50})` on mount.
- [ ] **Step 2:** Keep the create Modal as-is (out of scope for the table).
- [ ] **Step 3: `npm run check`** + **manual verify** via `run` skill: search,
  sort, filter, select+delete, row→drawer edit→save, pagination.
- [ ] **Step 4: Commit** — `feat(web): participants list on DataTable`

---

## Phase 6 — Migrate the remaining list pages (repeatable)

For each of: `invoices`, `estimates`, `custom-items`, `tax-rates`,
`plan-managers`, `recurring` — **one task each, same shape**:

- [ ] Backend: add `<Resource>Cols` spec + base SELECT const + `Query` + service
  `Query` + handler branch (mirror Tasks 3–4). Confirm scan column order against
  each table's migration.
- [ ] Frontend: rewrite the page on `<DataTable>` with that resource's columns +
  actions (mirror Task 7).
- [ ] `go test ./... && go vet ./... && gofmt -l . && cd web && npm run check`.
- [ ] Commit per resource: `feat(web): <resource> list on DataTable`.

> Do these one resource per commit so each is independently reviewable and
> revertible. Stop and surface anything whose columns don't map cleanly (computed
> columns, money formatting) rather than forcing it.

---

## Phase 7 — ShiftTable (highest risk) + cleanup

### Task 8: Decide + migrate or defer ShiftTable

ShiftTable is embedded (dashboard + participant detail), not a list route, and
likely has no `GET /api/shifts` query endpoint shaped like the others.

- [ ] **Step 1:** Assess: does a server query endpoint exist/make sense here? If
  not trivial, **defer** ShiftTable to a follow-up and keep `@careswitch/svelte-data-table`.
- [ ] **Step 2 (if migrating):** add `shift` listquery support + embed `<DataTable>`
  in both hosts; verify both.

### Task 9: Drop the unused dep

- [ ] **Step 1:** Once nothing imports `@careswitch/svelte-data-table`,
  `cd web && npm rm @careswitch/svelte-data-table`.
- [ ] **Step 2:** `npm run check && npm run build`.
- [ ] **Step 3: Commit** — `chore(web): drop client-side data-table dep`.

---

## Final gate

- [ ] `go test -race ./...` — green
- [ ] `go vet ./...` ; `gofmt -l .` — clean
- [ ] `CGO_ENABLED=0 go build ./cmd/tallyo` — builds
- [ ] `cd web && npm run check && npm run build` — clean
- [ ] Update `docs/data-model.md` only if a migration changed (none expected).
