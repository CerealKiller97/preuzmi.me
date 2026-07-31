// Package database holds the SQL schema for the app's SQLite store. The schema
// is embedded from schema.sql at build time so it is applied correctly no matter
// what the process working directory is.
//
// schema.sql is the single source of truth for table shape. Apply creates
// missing tables and ALTERs in any columns that appear in schema.sql but are
// absent from an older database — so adding a column means editing schema.sql
// only; no Go migration stub is required.
package database

import _ "embed"

// Schema is the DDL describing the desired tables. Edit schema.sql to change
// it; a rebuild picks up the change and Apply brings existing databases forward.
//
//go:embed schema.sql
var Schema string
