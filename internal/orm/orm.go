// Package orm is a lightweight data-access layer over database/sql.
//
// Usage mirrors common Go ORM / trpc-go DAO patterns (DB.WithContext, Create,
// Where/Find, Updates, Delete, Transaction) while keeping raw SQL available
// under the hood. When no *sql.DB is provided, Memory backend still exercises
// the same API so Sheets+PG dual paths stay testable without a cluster.
package orm

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DB is the entry point — wrap *sql.DB or use Memory().
type DB struct {
	sql    *sql.DB
	memory *memoryStore
	now    func() time.Time
}

// OpenSQL wraps an existing *sql.DB (e.g. from postgres.Store.DB()).
func OpenSQL(db *sql.DB) *DB {
	return &DB{sql: db, now: time.Now}
}

// Memory returns an in-process store with the same Session API.
func Memory() *DB {
	return &DB{memory: newMemoryStore(), now: time.Now}
}

// SQL exposes the underlying *sql.DB when present (nil for Memory).
func (d *DB) SQL() *sql.DB {
	if d == nil {
		return nil
	}
	return d.sql
}

// Backend reports "sql" or "memory".
func (d *DB) Backend() string {
	if d == nil {
		return "nil"
	}
	if d.sql != nil {
		return "sql"
	}
	return "memory"
}

// Session is a request-scoped handle (optional tenant filter, tx).
type Session struct {
	db       *DB
	ctx      context.Context
	tenantID string
	tx       *sql.Tx
}

// WithContext starts a session (trpc-go / GORM style).
func (d *DB) WithContext(ctx context.Context) *Session {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Session{db: d, ctx: ctx}
}

// Tenant scopes subsequent writes/reads to tenant_id (multi-tenant).
func (s *Session) Tenant(id string) *Session {
	s.tenantID = strings.TrimSpace(id)
	return s
}

// Transaction runs fn inside a SQL transaction. Memory backend runs fn
// directly (per-op locks already serialize); do not nest a second mutex.
func (d *DB) Transaction(ctx context.Context, fn func(*Session) error) error {
	s := d.WithContext(ctx)
	if d.sql == nil {
		return fn(s)
	}
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	s.tx = tx
	if err := fn(s); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Row is a generic record map (column → value). Prefer typed helpers for hot paths.
type Row map[string]any

// Create inserts a row into table. Columns come from Row keys.
// Auto-fills created_at / updated_at when absent; tenant_id when Session.Tenant set.
func (s *Session) Create(table string, row Row) error {
	if row == nil {
		return fmt.Errorf("orm: nil row")
	}
	row = cloneRow(row)
	now := s.db.now().UTC()
	if _, ok := row["created_at"]; !ok {
		row["created_at"] = now
	}
	if _, ok := row["updated_at"]; !ok {
		row["updated_at"] = now
	}
	if s.tenantID != "" {
		if _, ok := row["tenant_id"]; !ok {
			row["tenant_id"] = s.tenantID
		}
	}
	if s.db.sql == nil {
		return s.db.memory.insert(table, row)
	}
	cols, args := ordered(row)
	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoteIdent(table), strings.Join(quoteIdents(cols), ", "), strings.Join(placeholders, ", "))
	_, err := s.exec(q, args...)
	return err
}

// Query builds a chainable finder.
func (s *Session) Query(table string) *Query {
	return &Query{sess: s, table: table}
}

// Exec runs raw SQL (SQL backend only).
func (s *Session) Exec(query string, args ...any) (sql.Result, error) {
	if s.db.sql == nil {
		return nil, fmt.Errorf("orm: Exec requires SQL backend")
	}
	return s.exec(query, args...)
}

// RawQuery runs a SELECT and returns maps (SQL backend only).
func (s *Session) RawQuery(query string, args ...any) ([]Row, error) {
	if s.db.sql == nil {
		return nil, fmt.Errorf("orm: RawQuery requires SQL backend")
	}
	return s.queryRows(query, args...)
}

func (s *Session) exec(query string, args ...any) (sql.Result, error) {
	if s.tx != nil {
		return s.tx.ExecContext(s.ctx, query, args...)
	}
	return s.db.sql.ExecContext(s.ctx, query, args...)
}

func (s *Session) queryRows(query string, args ...any) ([]Row, error) {
	var rows *sql.Rows
	var err error
	if s.tx != nil {
		rows, err = s.tx.QueryContext(s.ctx, query, args...)
	} else {
		rows, err = s.db.sql.QueryContext(s.ctx, query, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []Row
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := Row{}
		for i, c := range cols {
			r[c] = normalize(vals[i])
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Query is a fluent finder: Where → Find / First / Updates / Delete.
type Query struct {
	sess   *Session
	table  string
	wheres []pred
	limit  int
	order  string
}

type pred struct {
	col string
	op  string
	val any
}

// Where adds equality (col = val). Call multiple times for AND.
func (q *Query) Where(col string, val any) *Query {
	q.wheres = append(q.wheres, pred{col: col, op: "=", val: val})
	return q
}

// WhereOp adds a comparison (e.g. ">", ">=").
func (q *Query) WhereOp(col, op string, val any) *Query {
	q.wheres = append(q.wheres, pred{col: col, op: op, val: val})
	return q
}

// Order sets ORDER BY clause (trusted identifier list, e.g. "created_at DESC").
func (q *Query) Order(expr string) *Query {
	q.order = expr
	return q
}

// Limit caps result rows.
func (q *Query) Limit(n int) *Query {
	q.limit = n
	return q
}

func (q *Query) withTenant() {
	if q.sess.tenantID == "" {
		return
	}
	for _, w := range q.wheres {
		if w.col == "tenant_id" {
			return
		}
	}
	q.wheres = append(q.wheres, pred{col: "tenant_id", op: "=", val: q.sess.tenantID})
}

// Find returns matching rows.
func (q *Query) Find() ([]Row, error) {
	q.withTenant()
	if q.sess.db.sql == nil {
		return q.sess.db.memory.find(q.table, q.wheres, q.order, q.limit)
	}
	sqlStr, args := q.buildSelect()
	return q.sess.queryRows(sqlStr, args...)
}

// First returns one row or sql.ErrNoRows.
func (q *Query) First() (Row, error) {
	q.limit = 1
	rows, err := q.Find()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return rows[0], nil
}

// Count returns matching row count.
func (q *Query) Count() (int, error) {
	q.withTenant()
	if q.sess.db.sql == nil {
		rows, err := q.sess.db.memory.find(q.table, q.wheres, "", 0)
		return len(rows), err
	}
	where, args := q.buildWhere(1)
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM %s%s", quoteIdent(q.table), where)
	var n int
	var err error
	if q.sess.tx != nil {
		err = q.sess.tx.QueryRowContext(q.sess.ctx, sqlStr, args...).Scan(&n)
	} else {
		err = q.sess.db.sql.QueryRowContext(q.sess.ctx, sqlStr, args...).Scan(&n)
	}
	return n, err
}

// Updates sets columns on matching rows.
func (q *Query) Updates(patch Row) (int64, error) {
	if len(patch) == 0 {
		return 0, nil
	}
	q.withTenant()
	patch = cloneRow(patch)
	if _, ok := patch["updated_at"]; !ok {
		patch["updated_at"] = q.sess.db.now().UTC()
	}
	if q.sess.db.sql == nil {
		return q.sess.db.memory.update(q.table, q.wheres, patch)
	}
	setCols, setArgs := ordered(patch)
	setParts := make([]string, len(setCols))
	for i, c := range setCols {
		setParts[i] = fmt.Sprintf("%s = $%d", quoteIdent(c), i+1)
	}
	where, whereArgs := q.buildWhere(len(setArgs) + 1)
	args := append(setArgs, whereArgs...)
	sqlStr := fmt.Sprintf("UPDATE %s SET %s%s", quoteIdent(q.table), strings.Join(setParts, ", "), where)
	res, err := q.sess.exec(sqlStr, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Delete removes matching rows.
func (q *Query) Delete() (int64, error) {
	q.withTenant()
	if q.sess.db.sql == nil {
		return q.sess.db.memory.delete(q.table, q.wheres)
	}
	where, args := q.buildWhere(1)
	sqlStr := fmt.Sprintf("DELETE FROM %s%s", quoteIdent(q.table), where)
	res, err := q.sess.exec(sqlStr, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (q *Query) buildSelect() (string, []any) {
	where, args := q.buildWhere(1)
	sqlStr := fmt.Sprintf("SELECT * FROM %s%s", quoteIdent(q.table), where)
	if q.order != "" {
		sqlStr += " ORDER BY " + q.order
	}
	if q.limit > 0 {
		sqlStr += fmt.Sprintf(" LIMIT %d", q.limit)
	}
	return sqlStr, args
}

func (q *Query) buildWhere(start int) (string, []any) {
	if len(q.wheres) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(q.wheres))
	args := make([]any, 0, len(q.wheres))
	i := start
	for _, w := range q.wheres {
		parts = append(parts, fmt.Sprintf("%s %s $%d", quoteIdent(w.col), w.op, i))
		args = append(args, w.val)
		i++
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func ordered(row Row) ([]string, []any) {
	cols := make([]string, 0, len(row))
	for k := range row {
		cols = append(cols, k)
	}
	// Stable-ish: sort for deterministic SQL (tests / plans).
	for i := 0; i < len(cols); i++ {
		for j := i + 1; j < len(cols); j++ {
			if cols[j] < cols[i] {
				cols[i], cols[j] = cols[j], cols[i]
			}
		}
	}
	args := make([]any, len(cols))
	for i, c := range cols {
		args[i] = row[c]
	}
	return cols, args
}

func cloneRow(r Row) Row {
	out := make(Row, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

func normalize(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.UTC()
	default:
		return v
	}
}

func quoteIdent(name string) string {
	name = strings.ReplaceAll(name, `"`, "")
	return `"` + name + `"`
}

func quoteIdents(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = quoteIdent(c)
	}
	return out
}

// --- memory backend ---

type memoryStore struct {
	mu     sync.Mutex
	tables map[string][]Row
	seq    map[string]int64
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		tables: map[string][]Row{},
		seq:    map[string]int64{},
	}
}

func (m *memoryStore) insert(table string, row Row) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := cloneRow(row)
	if _, ok := r["id"]; !ok {
		m.seq[table]++
		r["id"] = m.seq[table]
	}
	m.tables[table] = append(m.tables[table], r)
	return nil
}

func match(row Row, wheres []pred) bool {
	for _, w := range wheres {
		got, ok := row[w.col]
		if !ok {
			return false
		}
		switch w.op {
		case "=":
			if fmt.Sprint(got) != fmt.Sprint(w.val) {
				return false
			}
		case ">", ">=", "<", "<=":
			// best-effort numeric/string compare via Sprint for stub
			if !cmpSprint(got, w.val, w.op) {
				return false
			}
		default:
			if fmt.Sprint(got) != fmt.Sprint(w.val) {
				return false
			}
		}
	}
	return true
}

func cmpSprint(a, b any, op string) bool {
	as, bs := fmt.Sprint(a), fmt.Sprint(b)
	switch op {
	case ">":
		return as > bs
	case ">=":
		return as >= bs
	case "<":
		return as < bs
	case "<=":
		return as <= bs
	default:
		return as == bs
	}
}

func (m *memoryStore) find(table string, wheres []pred, _ string, limit int) ([]Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Row
	for _, r := range m.tables[table] {
		if match(r, wheres) {
			out = append(out, cloneRow(r))
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memoryStore) update(table string, wheres []pred, patch Row) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for i, r := range m.tables[table] {
		if match(r, wheres) {
			for k, v := range patch {
				r[k] = v
			}
			m.tables[table][i] = r
			n++
		}
	}
	return n, nil
}

func (m *memoryStore) delete(table string, wheres []pred) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.tables[table]
	dst := src[:0]
	var n int64
	for _, r := range src {
		if match(r, wheres) {
			n++
			continue
		}
		dst = append(dst, r)
	}
	m.tables[table] = dst
	return n, nil
}
