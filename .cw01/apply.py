from pathlib import Path

# Explicit, guarded edits on the isolated CW-01 branch. The final CI remains
# read-only; this temporary authoring helper is removed before delivery.
def edit(path, before, after):
    p = Path(path)
    text = p.read_text()
    count = text.count(before)
    if count != 1:
        raise SystemExit(f'{path}: expected one exact anchor, found {count}')
    p.write_text(text.replace(before, after, 1))

edit('internal/nlqexec/types.go',
     '\tpreviousResolutions []semantics.ClarificationResolution\n', '')
edit('internal/nlqexec/service.go',
     'func (s *Service) plan(ctx context.Context, e identity.Envelope, question QuestionRequest, operation, parent, action string) (PlanResult, error) {',
     'func (s *Service) plan(ctx context.Context, e identity.Envelope, question QuestionRequest, operation, parent, action string, previous ...semantics.ClarificationResolution) (PlanResult, error) {')
edit('internal/nlqexec/service.go',
     'sealClarificationCandidate(&candidate, validated, question.previousResolutions, admitted.route.Resolutions)',
     'sealClarificationCandidate(&candidate, validated, previous, admitted.route.Resolutions)')
edit('internal/nlqexec/service.go',
     'return s.plan(ctx, e, question, "", in.QueryID, "query.execute")',
     'return s.plan(ctx, e, question, "", in.QueryID, "query.execute", old.Route.Resolutions...)')
edit('internal/nlqexec/clarification.go',
     '\tout.previousResolutions = semantics.CloneClarificationResolutions(old.Route.Resolutions)\n', '')
