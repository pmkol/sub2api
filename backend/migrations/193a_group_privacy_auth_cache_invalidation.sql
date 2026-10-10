-- Keep privacy invalidation independent of the upstream group trigger so future
-- replacements of that function do not overwrite this additional watched field.
CREATE OR REPLACE FUNCTION enqueue_group_privacy_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.group_id = OLD.id
      AND k.deleted_at IS NULL
      AND k.key <> '';
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_groups_privacy_auth_cache_invalidation ON groups;
CREATE TRIGGER trg_groups_privacy_auth_cache_invalidation
AFTER UPDATE ON groups
FOR EACH ROW
WHEN (OLD.require_privacy_set IS DISTINCT FROM NEW.require_privacy_set)
EXECUTE FUNCTION enqueue_group_privacy_auth_cache_invalidation();
