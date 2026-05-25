-- Reverse 0005 — restore the previous CHECK list.
-- 'key_synced' rows will fail validation after this runs; only run
-- the down if you're confident no key_synced events exist (or
-- DELETE FROM analytics_events WHERE kind = 'key_synced' first).

ALTER TABLE analytics_events
  DROP CONSTRAINT analytics_events_kind_check;

ALTER TABLE analytics_events
  ADD CONSTRAINT analytics_events_kind_check
    CHECK (kind IN (
      'project_created',
      'first_key_synced',
      'first_translation_edited',
      'first_consumer_request',
      'first_ai_translation',
      'translation_edited',
      'consumer_request',
      'ai_translation'
    ));
