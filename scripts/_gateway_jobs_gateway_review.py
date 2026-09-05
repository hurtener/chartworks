from pathlib import Path
p=Path('internal/gateway/gateway.go');s=p.read_text();s=s.replace('\tRerank(context.Context, Call, *Budget, string, Candidates) (Ranked, error)','\tRerank(context.Context, Call, *Budget, string, Candidates) (Ranked, error)\n VisualRank(context.Context, Call, *Budget, string, Candidates) (Ranked, error)');p.write_text(s)
p=Path('internal/gateway/bifrost/engine.go');s=p.read_text();needle='func (e *Engine) Generate('
assert needle in s
s=s.replace(needle,'func (e *Engine) generate(',1)
s+='\n// Generate handles structured authoring and tool-free narratives. Visual ranking requires its sealed candidate method.\nfunc (e *Engine) Generate(ctx context.Context, call gateway.Call, b *gateway.Budget, name, system, prompt string, schema *gateway.Schema) (gateway.Generated, error) {\n if name=="visual_rank" {return gateway.Generated{},gateway.ErrInput}\n return e.generate(ctx,call,b,name,system,prompt,schema)\n}\n'
s=s.replace('if !call.Valid() || b == nil || !time.Now().Before(b.Deadline()) {','if b.Check(call)!=nil {',1)
p.write_text(s)
p=Path('internal/gateway/schema.go');s=p.read_text().replace('"io"','"io"\n "math"\n "strconv"',1)
s=s.replace('\tif delim, ok := v.(json.Delim); ok {','\tif number,ok:=v.(json.Number);ok {if len(number.String())>128{return ErrOutput};n,err:=strconv.ParseFloat(number.String(),64);if err!=nil||math.IsNaN(n)||math.IsInf(n,0){return ErrOutput}}\n\tif delim, ok := v.(json.Delim); ok {',1);p.write_text(s)
