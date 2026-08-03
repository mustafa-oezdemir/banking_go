-- This migration repairs an invariant owned by migration 000006. Rolling it
-- back must not remove the role column from otherwise current databases.
SELECT 1;
