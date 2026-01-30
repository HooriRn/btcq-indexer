package db

import _ "embed"

//go:embed ddl.sql
var dataDDL string

func CoreDDL() []string {
	return []string{SchemaCleanUp("midgard_agg"), SchemaCleanUp("midgard"), dataDDL}
}

// We have so many tables that dropping them all in one transaction makes us run out of locks
// (max_locks_per_transaction is too low). So, we drop the tables one-by-one in separate
// transactions.
func TableCleanup(schema string) string {
	return `
DO $$ DECLARE
	r RECORD;
BEGIN
	FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = '` + schema + `') LOOP
		EXECUTE 'DROP TABLE IF EXISTS ' || quote_ident(r.tablename) || ' CASCADE';
		COMMIT;
	END LOOP;
END $$;
`
}

func SchemaCleanUp(schema string) string {
	return `
DROP SCHEMA IF EXISTS ` + schema + ` CASCADE;
	`
}
