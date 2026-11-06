package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/store"
)

// sqlResult is the JSON response for SQL execution.
type sqlResult struct {
	Rows       []map[string]any `json:"rows"`
	Columns    []string         `json:"columns"`
	Affected   int64            `json:"affected_rows"`
	Error      string           `json:"error,omitempty"`
	IsQuery    bool             `json:"is_query,omitempty"`
}

// sqlExecAddon: POST /api/addons/{aid}/sql — execute SQL against an add-on database.
func (s *Server) sqlExecAddon(w http.ResponseWriter, r *http.Request, p *principal, a store.Addon, proj store.Project) {
	if a.Stopped {
		writeError(w, http.StatusConflict, "the database is stopped: start it first")
		return
	}
	k, ok := deploy.AddonKindByID(a.Kind)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown database kind")
		return
	}

	var in struct {
		SQL string `json:"sql"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.SQL) == "" {
		writeError(w, http.StatusBadRequest, "sql is required")
		return
	}

	res := s.execSQL(r.Context(), a.Kind, a.Kind, netJoinHostPort(a.Slug, proj.Namespace, k.Port), in.SQL)
	writeJSON(w, http.StatusOK, res)
}

// sqlExecApp: POST /api/apps/{id}/database/sql — execute SQL against an app-as-database.
func (s *Server) sqlExecApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if app.Database == nil {
		writeError(w, http.StatusBadRequest, "the app is not marked as a database")
		return
	}
	if app.Config.Replicas == 0 {
		writeError(w, http.StatusConflict, "the app is stopped: start it first")
		return
	}

	var in struct {
		SQL string `json:"sql"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.SQL) == "" {
		writeError(w, http.StatusBadRequest, "sql is required")
		return
	}

	port := deploy.AppDatabasePort(app)
	addr := netJoinHostPort(app.Slug, proj.Namespace, port)
	res := s.execSQL(r.Context(), app.Database.Engine, app.Slug, addr, in.SQL)
	writeJSON(w, http.StatusOK, res)
}

// execSQL runs SQL against a database and returns the result.
func (s *Server) execSQL(ctx context.Context, engine, dbName, addr, sql string) sqlResult {
	res := sqlResult{}

	// Connect to the database
	db, err := s.openDB(ctx, engine, addr)
	if err != nil {
		res.Error = fmt.Sprintf("connect: %v", err)
		return res
	}
	defer db.Close()

	// Determine if this is a query or a write
	isQuery := isQuerySQL(sql)

	// Execute the SQL
	if isQuery {
		res.IsQuery = true
		rows, cols, err := s.executeQuery(ctx, db, engine, sql)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		res.Columns = cols
		res.Rows = rows
	} else {
		result, err := db.ExecContext(ctx, sql)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		affected, _ := result.RowsAffected()
		res.Affected = affected
	}

	return res
}

// openDB opens a database connection for the given engine.
func (s *Server) openDB(ctx context.Context, engine, addr string) (*sql.DB, error) {
	var dsn string
	switch engine {
	case "postgres":
		// Read credentials from the cluster's secrets or use default
		dsn = fmt.Sprintf("host=%s port=5432 user=postgres sslmode=disable", addr)
	case "mysql", "mariadb":
		dsn = fmt.Sprintf("root@tcp(%s)/mysql", addr)
	case "redis":
		// Redis doesn't use database/sql; handle separately
		return nil, fmt.Errorf("SQL execution is not supported for Redis")
	default:
		return nil, fmt.Errorf("unsupported engine: %s", engine)
	}

	// Try to connect
	db, err := sql.Open(engine, dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(time.Minute)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

// executeQuery runs a SQL query and returns the rows and column names.
func (s *Server) executeQuery(ctx context.Context, db *sql.DB, engine, sql string) ([]map[string]any, []string, error) {
	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var results []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		valuePtrs := make([]any, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			val := values[i]
			// Convert []byte to string for JSON serialization
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return results, cols, nil
}

// isQuerySQL checks if the SQL looks like a SELECT/SHOW/DESCRIBE/etc. query.
func isQuerySQL(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	lower := strings.ToLower(trimmed)
	// If the SQL has multiple statements, check only the first
	if idx := strings.Index(lower, ";"); idx != -1 {
		lower = lower[:idx]
	}
	lower = strings.TrimSpace(lower)
	return strings.HasPrefix(lower, "select ") ||
		strings.HasPrefix(lower, "show ") ||
		strings.HasPrefix(lower, "describe ") ||
		strings.HasPrefix(lower, "desc ") ||
		strings.HasPrefix(lower, "explain ") ||
		strings.HasPrefix(lower, "with ") ||
		strings.HasPrefix(lower, "pragma ") ||
		strings.HasPrefix(lower, "pragma")
}

// netJoinHostPort joins host and namespace into a Kubernetes service address.
func netJoinHostPort(slug, namespace string, port int) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local:%d", slug, namespace, port)
}
