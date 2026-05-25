-- Add 'key_synced' to the analytics_events kind whitelist.
--
-- Migration 0004 enumerated kinds that included pre-derived
-- "first_X" variants the writer doesn't actually emit (the funnel
-- query derives firstAt via MIN(occurred_at) instead). The CLI's
-- key-scan handler needs a 'key_synced' kind so the admin Metrics
-- tab can compute time-to-first-key-sync, but the existing CHECK
-- rejects it. Replace the constraint with a list that includes
-- 'key_synced'; the obsolete first_X kinds stay in the list so any
-- already-written rows (none in practice today) keep validating.

ALTER TABLE analytics_events
  DROP CONSTRAINT analytics_events_kind_check;

ALTER TABLE analytics_events
  ADD CONSTRAINT analytics_events_kind_check
    CHECK (kind IN (
      'project_created',
      'key_synced',
      'translation_edited',
      'consumer_request',
      'ai_translation',
      'first_key_synced',
      'first_translation_edited',
      'first_consumer_request',
      'first_ai_translation'
    ));
