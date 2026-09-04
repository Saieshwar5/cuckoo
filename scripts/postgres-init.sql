-- Runs once, when the Postgres volume is first created.
--
-- The dev database (cuckoo) is created by the postgres image from POSTGRES_DB.
-- Tests run against a separate database so a `make test` can never touch data
-- you are looking at in the app.

CREATE DATABASE cuckoo_test OWNER cuckoo;
