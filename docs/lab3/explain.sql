BEGIN;
INSERT INTO trips(id,user_id,driver_id,start_latitude,start_longitude,end_latitude,end_longitude,price,status,started_at,finished_at,last_position_at)
SELECT gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),0,0,1,1,100,
CASE WHEN n%3=0 THEN 'completed' ELSE 'active' END,
now() - interval '2 days' + n * interval '1 second',
CASE WHEN n%3=0 THEN now() - interval '1 day' END,
CASE WHEN n%3=1 THEN clock_timestamp() ELSE NULL END
FROM generate_series(1,6000) n;
ANALYZE trips;
SELECT count(*) AS trips, count(*) FILTER(WHERE status='active') AS active FROM trips;
PREPARE first_page(timestamptz,int,double precision) AS
SELECT id, driver_id, COALESCE(last_position_at, started_at) AS position_at, updated_at,
       COALESCE($1::timestamptz, statement_timestamp() - make_interval(secs => $3)) AS cutoff
FROM trips
WHERE status='active' AND COALESCE(last_position_at,started_at) < COALESCE($1::timestamptz, statement_timestamp() - make_interval(secs => $3))
ORDER BY COALESCE(last_position_at,started_at),id LIMIT $2;
EXPLAIN (ANALYZE,BUFFERS) EXECUTE first_page(NULL,100,60);
CREATE TEMP TABLE first_batch ON COMMIT DROP AS EXECUTE first_page(NULL,100,60);
SELECT position_at AS cursor_at, id AS cursor_id, cutoff FROM first_batch
ORDER BY position_at DESC, id DESC LIMIT 1 \gset
PREPARE next_page(timestamptz,int,double precision,timestamptz,uuid) AS
SELECT id, driver_id, COALESCE(last_position_at, started_at) AS position_at, updated_at,
       COALESCE($1::timestamptz, statement_timestamp() - make_interval(secs => $3)) AS cutoff
FROM trips
WHERE status='active' AND COALESCE(last_position_at,started_at) < COALESCE($1::timestamptz, statement_timestamp() - make_interval(secs => $3))
AND (COALESCE(last_position_at,started_at),id) > ($4,$5)
ORDER BY COALESCE(last_position_at,started_at),id LIMIT $2;
EXPLAIN (ANALYZE,BUFFERS) EXECUTE next_page(:'cutoff',100,60,:'cursor_at',:'cursor_id');
ROLLBACK;
