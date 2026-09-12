"""Temporary branch-only patch. Remove this aid before the final PR."""
from pathlib import Path
import subprocess

path = Path('sdk/chartworks/reporting_delivery.go')
text = path.read_text()
text = text.replace('// ReportingRunRequest explicitly', '// ReportingDeliveryRunRequest explicitly')
text = text.replace('type ReportingRunRequest =', 'type ReportingDeliveryRunRequest =')
text = text.replace('in ReportingRunRequest)', 'in ReportingDeliveryRunRequest)')
text = text.replace('// ListReportingRuns reads', '// SearchReportingRuns reads')
text = text.replace('func (c *Client) ListReportingRuns(', 'func (c *Client) SearchReportingRuns(')
path.write_text(text)
changed = [str(path)]
for path in Path('test/acceptance').glob('phase31*.go'):
    text = path.read_text()
    new = text.replace('cw.ReportingRunRequest', 'cw.ReportingDeliveryRunRequest').replace('.ListReportingRuns(', '.SearchReportingRuns(')
    if new != text:
        path.write_text(new)
        changed.append(str(path))
subprocess.run(['gofmt', '-w', *changed], check=True)
subprocess.run(['git', 'diff', '--check'], check=True)
