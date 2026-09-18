-- Retained clarification replay/shadow evidence shares the existing immutable,
-- tenant-composite comparison identity and exact topic/rule foreign keys.
ALTER TABLE chartworks.topic_rule_comparison_evidence
 ADD COLUMN baseline_clarification_result jsonb,
 ADD COLUMN candidate_clarification_result jsonb;
ALTER TABLE chartworks.topic_rule_comparison_evidence
 ADD CONSTRAINT baseline_clarification_bounded CHECK (
  baseline_clarification_result IS NULL OR baseline_clarification_result = 'null'::jsonb OR
  (jsonb_typeof(baseline_clarification_result) = 'array' AND
   jsonb_array_length(baseline_clarification_result) <= 16 AND
   octet_length(baseline_clarification_result::text) <= 1048576)),
 ADD CONSTRAINT candidate_clarification_bounded CHECK (
  candidate_clarification_result IS NULL OR candidate_clarification_result = 'null'::jsonb OR
  (jsonb_typeof(candidate_clarification_result) = 'array' AND
   jsonb_array_length(candidate_clarification_result) <= 16 AND
   octet_length(candidate_clarification_result::text) <= 1048576));
