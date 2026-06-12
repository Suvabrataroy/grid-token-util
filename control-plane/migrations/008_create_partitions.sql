-- 008_create_partitions.sql
-- Creates monthly range partitions for the tasks and audit_log tables for the
-- current month and the next 3 months.  The default partitions created in
-- migrations 003 and 005 act as catch-alls for any rows that fall outside an
-- explicit partition.

DO $$
DECLARE
    start_date     DATE;
    end_date       DATE;
    partition_name TEXT;
    i              INT;
BEGIN
    FOR i IN 0..3 LOOP
        -- ── tasks partitions ─────────────────────────────────────────────────
        start_date     := DATE_TRUNC('month', CURRENT_DATE + (i || ' months')::INTERVAL);
        end_date       := start_date + INTERVAL '1 month';
        partition_name := 'tasks_' || TO_CHAR(start_date, 'YYYY_MM');

        IF NOT EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE c.relname = partition_name
              AND n.nspname = 'public'
        ) THEN
            EXECUTE FORMAT(
                'CREATE TABLE %I PARTITION OF tasks '
                'FOR VALUES FROM (%L) TO (%L)',
                partition_name, start_date, end_date
            );
            RAISE NOTICE 'Created tasks partition: %', partition_name;
        END IF;

        -- ── audit_log partitions ─────────────────────────────────────────────
        partition_name := 'audit_log_' || TO_CHAR(start_date, 'YYYY_MM');

        IF NOT EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE c.relname = partition_name
              AND n.nspname = 'public'
        ) THEN
            EXECUTE FORMAT(
                'CREATE TABLE %I PARTITION OF audit_log '
                'FOR VALUES FROM (%L) TO (%L)',
                partition_name, start_date, end_date
            );
            RAISE NOTICE 'Created audit_log partition: %', partition_name;
        END IF;
    END LOOP;
END;
$$;
