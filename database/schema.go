// Package database holds the SQL schema for the app's SQLite store. The schema
// is embedded from schema.sql at build time so it is applied correctly no matter
// what the process working directory is.
package database

import _ "embed"

// Schema is the DDL applied on startup to create the receipts table if it does
// not already exist. Edit schema.sql to change it; a rebuild picks up the change.
//
//go:embed schema.sql
var Schema string
