DO $$
DECLARE
    conflict_report TEXT;
BEGIN
    IF to_regclass('public.users') IS NULL THEN
        RAISE NOTICE 'users table does not exist, skipping email normalization migration';
        RETURN;
    END IF;

    LOCK TABLE users IN ACCESS EXCLUSIVE MODE;

    SELECT string_agg(
               format('%s <= [%s]', normalized_email, originals),
               E'\n'
           )
      INTO conflict_report
      FROM (
          SELECT
              LOWER(BTRIM(email)) AS normalized_email,
              string_agg(email, ', ' ORDER BY email) AS originals
          FROM users
          GROUP BY 1
          HAVING COUNT(*) > 1
      ) collisions;

    IF conflict_report IS NOT NULL THEN
        RAISE EXCEPTION
            'Cannot normalize users.email because normalized values would collide:%',
            E'\n' || conflict_report;
    END IF;

    UPDATE users
       SET email = LOWER(BTRIM(email))
     WHERE email <> LOWER(BTRIM(email));

    ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
    DROP INDEX IF EXISTS uq_users_email_lower;
    CREATE UNIQUE INDEX uq_users_email_lower ON users ((LOWER(email)));
END $$;
