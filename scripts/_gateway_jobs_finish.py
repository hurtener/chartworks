# One-time transport of the locally tested diff; removed before the PR.
import base64
import hashlib
import lzma
import pathlib
import subprocess

expected = '76441b9a8fefcfac6699a2b04c7079cb270f5624'
commit = subprocess.check_output(['git', 'cat-file', '-p', 'HEAD'], text=True)
assert 'parent ' + expected in commit.split('\n\n', 1)[0].splitlines()
parts = [pathlib.Path('scripts/_review_patch_%d.txt' % i) for i in range(4)]
patch = lzma.decompress(base64.b64decode(''.join(p.read_text() for p in parts), validate=True))
assert hashlib.sha256(patch).hexdigest() == '4855ff3211bffd4a88a1acccaf13dcd2e1081a9a3ad7232ba815fbd35d23468f'
subprocess.run(['git', 'apply', '--check', '-'], input=patch, check=True)
subprocess.run(['git', 'apply', '-'], input=patch, check=True)
subprocess.run(['git', 'rm', '--', *[str(p) for p in parts]], check=True)
