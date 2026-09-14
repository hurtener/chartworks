"""Temporary branch-only editing aid; removed before final review."""
from pathlib import Path
import subprocess

p = Path('web/report-viewer/component.test.mjs')
s = p.read_text()
old = "await evaluate(`context({theme:'light',locale:'en-US'});show(fixture.view)`);await waitTitle(fixtures.view.summary.target.id);"
new = old + "\n    // The run title is unchanged by locale updates. Wait for the actual translated\n    // control, rather than treating an already-present title as an acknowledgement.\n    await until(()=>evaluate(`${doc}.documentElement.lang==='en-US'&&Array.from(${body}.querySelectorAll('details > summary')).some(s=>s.textContent==='Run with different filters')`),'English filter controls were not rendered');"
if new not in s:
    if s.count(old) != 1: raise RuntimeError('viewer locale transition anchor changed')
    p.write_text(s.replace(old, new))
subprocess.run(['node', '--check', str(p)], check=True)
subprocess.run(['git', 'diff', '--check'], check=True)
