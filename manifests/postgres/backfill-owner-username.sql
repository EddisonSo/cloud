-- Backfill owner_username from users table
-- Run this ONCE while still on shared database before migration
-- This updates existing containers with their owner's username

UPDATE containers c
SET owner_username = COALESCE(u.username, '')
FROM users u
WHERE c.user_id = u.id
  AND (c.owner_username = '' OR c.owner_username IS NULL);

-- Verify the backfill
SELECT
    c.id,
    c.user_id,
    c.owner_username,
    c.name
FROM containers c
WHERE c.owner_username = '' OR c.owner_username IS NULL;
