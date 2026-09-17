from pathlib import Path
import json


def edit(path,before,after,count=1):
 p=Path(path);text=p.read_text()
 if text.count(before)!=count:raise SystemExit(f'{path}: changed anchor {text.count(before)}')
 p.write_text(text.replace(before,after))

# These two migrations are unmerged CW-01 work. Keep their SQL bytes unchanged
# and place them after the already shipped 035 migration from current main.
for old,new in [
 ('internal/store/postgres/migrations/036_clarification_comparison.sql','internal/store/postgres/migrations/037_clarification_comparison.sql'),
 ('internal/store/postgres/migrations/035_nlq_clarification.sql','internal/store/postgres/migrations/036_nlq_clarification.sql')]:
 if Path(new).exists():raise SystemExit(f'{new}: refusing migration overwrite')
 Path(old).rename(new)

edit('internal/store/postgres/errors_test.go','SchemaVersion() != "35" || len(manifest) != 35','SchemaVersion() != "37" || len(manifest) != 37')
edit('internal/store/postgres/errors_test.go','''		{34, "migrations/035_reporting_output_intent.sql", "block_output_intents_check"},''','''		{34, "migrations/035_reporting_output_intent.sql", "block_output_intents_check"},
  {35,"migrations/036_nlq_clarification.sql","nlq_clarification_immutable"},
  {36,"migrations/037_clarification_comparison.sql","baseline_clarification_bounded"},''')
edit('internal/topicapi/http_test.go','len(r.Definitions()) != 32','len(r.Definitions()) != 35')
p=Path('docs/contracts/chartworks-topic-draft-operations.json');manifest=json.loads(p.read_text())
if len(manifest)!=32:raise SystemExit('topic operation inventory changed')
for suffix,action,effect in [
 ('preview','topics.write','deterministic_draft_preview'),
 ('export','topics.export','retained_metadata_export'),
 ('import-preview','topics.write','deterministic_import_preview')]:
 path='/v1/topics/{id}/clarifications/'+suffix
 if any(x['path']==path for x in manifest):raise SystemExit('duplicate operation')
 manifest.append({'method':'POST','path':path,'action':action,'effect':effect})
p.write_text(json.dumps(manifest,indent=2)+'\n')

edit('internal/nlqroute/service_test.go','''	return rulesets.Evaluation{Result: semantics.ConstraintEvaluation{Allowed: true}}, nil''',''' return rulesets.Evaluation{Topic:r.published.Definition.Topic,TopicVersion:r.published.Definition.TopicVersion,PackDigest:r.published.Definition.PackDigest,RuleVersion:r.published.Definition.Version,RuleDigest:r.published.Digest,Result:semantics.ConstraintEvaluation{Allowed:true}},nil''')
# Null-inclusion assertion checks the actual reviewed OR predicate position.
edit('internal/exec/business_sql_test.go','!strings.Contains(out.SQL, "IS NULL OR")','!strings.Contains(out.SQL, `OR "sales"."amount" IS NULL`)')
edit('internal/exec/business_sql_test.go','strings.Contains(out.SQL, "IS NULL OR")','strings.Contains(out.SQL, "IS NULL")')
