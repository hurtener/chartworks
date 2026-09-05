from pathlib import Path
p=Path('internal/config/config.go');s=p.read_text()
s=s.replace('type Provider struct {','type Provider struct {\n Type string `json:"type,omitempty"`')
s=s.replace('type Role struct {','type Role struct {\n ModelRevision string `json:"model_revision,omitempty"`')
s=s.replace('type Gateway struct {','type Gateway struct {\n Limits GatewayLimits `json:"limits"`')
s=s.replace('Gateway{Driver: "bifrost",','Gateway{Limits: DefaultGatewayLimits(), Driver: "bifrost",')
s=s.replace('if v.Features.Gateway || v.Features.MCP','if v.Features.MCP')
a=s.index('\tif v.Gateway.Driver != "bifrost" {');b=s.index('\nfunc checkJSON',a)
s=s[:a]+'\treturn ValidateGateway(v.Gateway, v.Features.Gateway)\n}\n'+s[b:]
p.write_text(s)
p=Path('go.mod');s=p.read_text().replace('require (','require (\n github.com/maximhq/bifrost/core v1.6.2\n github.com/santhosh-tekuri/jsonschema/v5 v5.3.1\n github.com/robfig/cron/v3 v3.0.1',1);p.write_text(s)
