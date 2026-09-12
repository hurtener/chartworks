"""Temporary branch-only editing aid; remove before submitting the final PR."""
from pathlib import Path
import subprocess


def replace(path, old, new):
    file = Path(path)
    text = file.read_text()
    if new in text:
        return
    if text.count(old) != 1:
        raise RuntimeError(f"Expected exactly one edit anchor in {path}")
    file.write_text(text.replace(old, new, 1))


replace("internal/config/reporting.go", "type Reporting struct {\n", "type Reporting struct {\n\tViewer ReportingViewer `json:\"viewer\"`\n")
replace("internal/config/reporting.go", "return Reporting{Execution:", "return Reporting{Viewer: DefaultReportingViewer(), Execution:")
replace("internal/config/reporting.go", "func (c Reporting) Validate() error {\n", "func (c Reporting) Validate() error {\n\tif err := c.Viewer.Validate(); err != nil {\n\t\treturn err\n\t}\n")
file = Path("internal/reporting/delivery_model.go")
text = file.read_text()
if '*RetainedOutput' in text:
    text = text.replace('*RetainedOutput', '*ViewerOutput')
    text = '\n'.join(line for line in text.split('\n') if '*ResultPage' not in line)
    file.write_text(text)

files = subprocess.check_output(['git', 'ls-files', '*.go'], text=True).splitlines()
subprocess.run(['gofmt', '-w', *files], check=True)
