from pathlib import Path


def replace(path, old, new):
    p = Path(path)
    text = p.read_text()
    if new in text:
        return
    if text.count(old) != 1:
        raise SystemExit(f"Expected one integration anchor in {path}")
    p.write_text(text.replace(old, new))


replace('internal/config/config.go', 'type Values struct {\n', 'type Values struct {\n\tUploads Uploads `json:"uploads"`\n\tProfiling Profiling `json:"profiling"`\n')
replace('internal/config/config.go', '\tv.Sources = c.values.Sources.Clone()\n', '\tv.Sources = c.values.Sources.Clone()\n\tv.Uploads = c.values.Uploads.Clone()\n\tv.Profiling = c.values.Profiling.Clone()\n')
replace('internal/config/config.go', '\t\tSources:   DefaultSources(),', '\t\tUploads: DefaultUploads(),\n\t\tProfiling: DefaultProfiling(),\n\t\tSources:   DefaultSources(),')
replace('internal/config/config.go', '\treturn ValidateGateway(v.Gateway, v.Features.Gateway)\n', '\tif err := ValidateUploads(v.Uploads); err != nil { return err }\n\tif err := ValidateProfiling(v.Profiling); err != nil { return err }\n\tif (v.Uploads.Enabled || v.Profiling.Enabled) && !v.Sources.Enabled { return invalid("engineering", "source access must be explicitly enabled") }\n\treturn ValidateGateway(v.Gateway, v.Features.Gateway)\n')
replace('internal/engineering/parse_xlsx.go', '\t"io"\n', '\t"io"\n\t"io/fs"\n')
replace('internal/engineering/parse_xlsx.go', 'f.Mode()&060000!=0', 'f.Mode()&fs.ModeType!=0 && !f.FileInfo().IsDir()')
