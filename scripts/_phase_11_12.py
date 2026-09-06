from pathlib import Path

# Previous integration edits are committed. Apply only this next reviewed delta.
p=Path('internal/engineering/uploads.go')
t=p.read_text().replace('*gateway.Engine','gateway.Engine')
if 'summarySchema *gateway.Schema' not in t and 'summarySchema  *gateway.Schema' not in t:
    t=t.replace('type Service struct {','type Service struct {\n\tsummarySchema *gateway.Schema')
if 'schema, err := profileSummarySchema()' not in t:
    t=t.replace('\tctx, cancel := context.WithCancel(context.Background())','\tschema, err := profileSummarySchema()\n\tif err != nil { return nil, err }\n\tctx, cancel := context.WithCancel(context.Background())')
    t=t.replace('return &Service{repo: repo,','return &Service{summarySchema:schema,repo: repo,')
p.write_text(t)
