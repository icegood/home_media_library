package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"media-library/backend/internal/applog"
)

// loggingDriver wraps a database/sql driver so every Exec and Query statement
// is written to the app log at debug level. This makes the log file show the
// store's activity during normal use. Query logging only activates when the
// configured log level is Debug.
type loggingDriver struct {
	mu    sync.Mutex
	inner driver.Driver
	label string
}

var (
	loggedSQLite   = &loggingDriver{label: "sqlite"}
	loggedPostgres = &loggingDriver{label: "postgres"}
)

func init() {
	sql.Register("sqlite-logged", loggedSQLite)
	sql.Register("pgx-logged", loggedPostgres)
}

func (d *loggingDriver) set(inner driver.Driver) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inner = inner
}

func (d *loggingDriver) Open(dsn string) (driver.Conn, error) {
	d.mu.Lock()
	inner := d.inner
	d.mu.Unlock()
	if inner == nil {
		return nil, errors.New("logging driver has no underlying driver")
	}
	conn, err := inner.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &loggingConn{conn: conn, label: d.label}, nil
}

// openLogged opens a database through the applog-tracing wrapper so all SQL
// traffic lands in the same log file the admin Logs page reads.
func openLogged(driverName, dsn string) (*sql.DB, error) {
	var (
		inner   driver.Driver
		wrapper *loggingDriver
		logged  string
	)
	switch driverName {
	case "sqlite":
		raw, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, err
		}
		inner, logged = raw.Driver(), "sqlite-logged"
		_ = raw.Close()
		wrapper = loggedSQLite
	case "pgx":
		raw, err := sql.Open("pgx", dsn)
		if err != nil {
			return nil, err
		}
		inner, logged = raw.Driver(), "pgx-logged"
		_ = raw.Close()
		wrapper = loggedPostgres
	default:
		return nil, fmt.Errorf("no logging wrapper for driver %q", driverName)
	}
	wrapper.set(inner)
	return sql.Open(logged, dsn)
}

type loggingConn struct {
	conn  driver.Conn
	label string
}

var querySeq uint64

func (c *loggingConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &loggingStmt{stmt: stmt, label: c.label, query: query}, nil
}

func (c *loggingConn) Close() error { return c.conn.Close() }

func (c *loggingConn) Begin() (driver.Tx, error) { return c.conn.Begin() }

func (c *loggingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if conn, ok := c.conn.(driver.ConnPrepareContext); ok {
		stmt, err := conn.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		return &loggingStmt{stmt: stmt, label: c.label, query: query}, nil
	}
	return c.Prepare(query)
}

func (c *loggingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if conn, ok := c.conn.(driver.ConnBeginTx); ok {
		return conn.BeginTx(ctx, opts)
	}
	return c.Begin()
}

func (c *loggingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	id := atomic.AddUint64(&querySeq, 1)
	applog.Printf(applog.Debug, "db %s [%d]: %s", c.label, id, query)
	start := time.Now()
	if execer, ok := c.conn.(driver.ExecerContext); ok {
		result, err := execer.ExecContext(ctx, query, args)
		logDone(c.label, id, start, err)
		return result, err
	}
	stmt, err := c.PrepareContext(ctx, query)
	if err != nil {
		logDone(c.label, id, start, err)
		return nil, err
	}
	defer stmt.Close()
	if execer, ok := stmt.(driver.StmtExecContext); ok {
		return execer.ExecContext(ctx, args)
	}
	return stmt.Exec(namedValues(args))
}

func (c *loggingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	id := atomic.AddUint64(&querySeq, 1)
	applog.Printf(applog.Debug, "db %s [%d]: %s", c.label, id, query)
	start := time.Now()
	if queryer, ok := c.conn.(driver.QueryerContext); ok {
		rows, err := queryer.QueryContext(ctx, query, args)
		logDone(c.label, id, start, err)
		return rows, err
	}
	stmt, err := c.PrepareContext(ctx, query)
	if err != nil {
		logDone(c.label, id, start, err)
		return nil, err
	}
	defer stmt.Close()
	if queryer, ok := stmt.(driver.StmtQueryContext); ok {
		return queryer.QueryContext(ctx, args)
	}
	return stmt.Query(namedValues(args))
}

type loggingStmt struct {
	stmt  driver.Stmt
	label string
	query string
}

func (s *loggingStmt) Close() error              { return s.stmt.Close() }
func (s *loggingStmt) NumInput() int             { return s.stmt.NumInput() }
func (s *loggingStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.stmt.Exec(args)
}
func (s *loggingStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.stmt.Query(args)
}

func (s *loggingStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	id := atomic.AddUint64(&querySeq, 1)
	applog.Printf(applog.Debug, "db %s [%d]: %s", s.label, id, s.query)
	start := time.Now()
	if execer, ok := s.stmt.(driver.StmtExecContext); ok {
		result, err := execer.ExecContext(ctx, args)
		logDone(s.label, id, start, err)
		return result, err
	}
	return s.Exec(namedValues(args))
}

func (s *loggingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	id := atomic.AddUint64(&querySeq, 1)
	applog.Printf(applog.Debug, "db %s [%d]: %s", s.label, id, s.query)
	start := time.Now()
	if queryer, ok := s.stmt.(driver.StmtQueryContext); ok {
		rows, err := queryer.QueryContext(ctx, args)
		logDone(s.label, id, start, err)
		return rows, err
	}
	return s.Query(namedValues(args))
}

func namedValues(args []driver.NamedValue) []driver.Value {
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return values
}

func logDone(label string, id uint64, start time.Time, err error) {
	duration := time.Since(start).Round(time.Millisecond)
	if err != nil {
		applog.Printf(applog.Debug, "db %s [%d]: done (%s): %v", label, id, duration, err)
		return
	}
	applog.Printf(applog.Debug, "db %s [%d]: done (%s)", label, id, duration)
}
