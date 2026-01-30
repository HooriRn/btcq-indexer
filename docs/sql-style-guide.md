# Guidelines for writing SQL in Midgard

These are our preferred ways of writing SQL queries/expressions. The goal is to make the code
consistent and readable. Try to follow them for all new code whenever possible.

## Upper/lower case

The idea is very simple:

- Everything that is an SQL keyword should be uppercase
- Everything else should be lowercase. This includes, but is not limited to: schemas, relations,
  columns, functions, types… Both user created and ones that come with Postgres.

For some things it can be unclear whether they are keywords or just regular functions, most notably
`COALESCE`, `GREATEST`, `LEAST`. When in doubt, check whether Postgres recognizes it as a function
(compare `\df min` and `\df coalesce` in `psql`) or check the keywords list:
https://www.postgresql.org/docs/current/sql-keywords-appendix.html

This guideline differs the most from the common usage in the case of types: we use `bigint`
(and not `BIGINT`).

## Types

Use short one-word synonyms instead of multi word type names: `timestamptz` instead of
`timestamp with time zone`, `varbit` instead of `bit varying`, etc.

Don't user `char(x)`, `varchar(x)`, or `varchar`, use `text`.

## Be explicit

When choosing between explicit or implicit syntax, prefer explicit. In particular:

Always use `AS` when aliasing a column or a table name/expression. It makes it easier to read,
especially with complicated expressions.

Prefer column names in `GROUP BY`, don't use column numbers whenever possible.

Prefer explicit joins, it makes it easier to understand what kind of join this is.

## Further resources

The author of these guidelines (huginn) generally endorses the following, more detailed, style
guide: https://docs.telemetry.mozilla.org/concepts/sql_style.html
