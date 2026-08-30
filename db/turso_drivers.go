//go:build !golibsql

package db

// This file is retained for the variant matrix documentation and the
// non-golibsql build. The remote driver implementations (TursoServerlessOpen,
// LibsqlOpen, TursoSyncOpen) live in turso_remote.go (available in all
// builds). go-libsql (turso_golibsql.go) is exclusive to the `golibsql` build
// because it registers the same driver name "libsql" as libsql-client-go.
