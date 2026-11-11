-- A student can have at most one payment per collection. Earlier versions could
-- record a repeated payment twice: keep the earliest of each pair and return the
-- rest to manual review instead of deleting them.
UPDATE payments
SET student_id = NULL, collection_id = NULL, match_method = 'unmatched'
WHERE student_id IS NOT NULL AND collection_id IS NOT NULL
AND id NOT IN (
    SELECT MIN(id) FROM payments
    WHERE student_id IS NOT NULL AND collection_id IS NOT NULL
    GROUP BY student_id, collection_id
);

CREATE UNIQUE INDEX IF NOT EXISTS payments_student_collection_unique
ON payments(student_id, collection_id)
WHERE student_id IS NOT NULL AND collection_id IS NOT NULL;
