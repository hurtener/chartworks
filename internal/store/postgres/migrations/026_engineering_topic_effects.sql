ALTER TABLE chartworks.engineering_proposal_effects
 DROP CONSTRAINT engineering_proposal_effects_kind_check;
ALTER TABLE chartworks.engineering_proposal_effects
 ADD CONSTRAINT engineering_proposal_effects_kind_check
 CHECK(kind IN('pipeline_draft','pipeline_publication','pipeline_run','managed_step','compensation','topic_draft'));
ALTER TABLE chartworks.engineering_proposal_effects
 DROP CONSTRAINT engineering_proposal_effects_observed_order_check;
ALTER TABLE chartworks.engineering_proposal_effects
 ADD CONSTRAINT engineering_proposal_effects_observed_order_check CHECK(observed_order BETWEEN 0 AND 5);
ALTER TABLE chartworks.engineering_proposal_references
 DROP CONSTRAINT engineering_proposal_references_check;
ALTER TABLE chartworks.engineering_proposal_references
 ADD CONSTRAINT engineering_proposal_references_check
 CHECK((kind,permission) IN(('source','query'),('dataset','query'),('execution_context','use'),('topic','read')));
