// Build-only tooling. Production embeds generated/report-app.js and never loads npm.
import {createHash} from 'node:crypto';
import {mkdir, readFile, writeFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import {dirname, relative, resolve, sep} from 'node:path';
import {fileURLToPath} from 'node:url';

const require = createRequire(import.meta.url);
const esbuild = require('esbuild');
const directory = dirname(fileURLToPath(import.meta.url));
const repository = resolve(directory, '../..');
const entry = 'web/report-app/app.js';
const output = 'web/report-app/generated/report-app.js';
const manifestPath = 'web/report-app/generated/manifest.json';
const configuration = ['web/report-app/build.mjs', 'web/report-app/package.json', 'web/report-app/package-lock.json'];
const styles = ['web/report-viewer/styles.css', 'web/report-app/styles.css'];
const check = process.argv.length === 3 && process.argv[2] === '--check';
if (process.argv.length !== (check ? 3 : 2)) throw new Error('Usage: node build.mjs [--check]');

const packageJSON = JSON.parse(await readFile(resolve(directory, 'package.json'), 'utf8'));
const lock = JSON.parse(await readFile(resolve(directory, 'package-lock.json'), 'utf8'));
const version = packageJSON.devDependencies.esbuild;
if (!/^\d+\.\d+\.\d+$/.test(version) || esbuild.version !== version ||
    lock.packages[''].devDependencies.esbuild !== version || lock.packages['node_modules/esbuild'].version !== version) {
  throw new Error('Install the exact locked build tool with npm ci before regenerating assets');
}
const repositoryPath = path => {
  const value = relative(repository, resolve(repository, path)).split(sep).join('/');
  if (!value || value.startsWith('../') || value.startsWith('/') || value.includes('node_modules/')) {
    throw new Error(`Non-authored or out-of-repository bundle input: ${path}`);
  }
  return value;
};
const digest = (path, bytes) => ({path, bytes: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex')});
const fileDigest = async path => digest(path, await readFile(resolve(repository, path)));
const result = await esbuild.build({
  absWorkingDir: repository,
  entryPoints: [entry],
  outfile: output,
  bundle: true,
  format: 'iife',
  platform: 'browser',
  target: ['es2022'],
  charset: 'utf8',
  minify: true,
  // Only opted-in disposable UI state names are shortened. Wire, domain and
  // host-bridge properties keep their original names. The prefix is reserved.
  mangleProps: /^_ui[A-Z]/,
  treeShaking: true,
  sourcemap: false,
  legalComments: 'none',
  metafile: true,
  write: false,
  logLevel: 'silent',
});
const emitted = result.outputFiles;
const bundled = result.metafile.outputs[output];
if (emitted.length !== 1 || repositoryPath(emitted[0].path) !== output || !bundled ||
    bundled.entryPoint !== entry || bundled.imports.length !== 0) {
  throw new Error('The report app must be one self-contained JavaScript resource, without runtime imports or chunks');
}
const inputs = await Promise.all(Object.entries(result.metafile.inputs).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(async ([path, value]) => {
  path = repositoryPath(path);
  if (!path.endsWith('.js') || value.imports.some(item => item.external || item.kind !== 'import-statement')) {
    throw new Error(`Only local authored JavaScript modules and static imports are allowed: ${path}`);
  }
  return {
    ...await fileDigest(path),
    imports: [...new Set(value.imports.map(item => repositoryPath(item.path)))].sort(),
    bytesInOutput: bundled.inputs[path]?.bytesInOutput ?? 0,
  };
}));
if (inputs.filter(input => input.path === 'web/report-viewer/presentation.js' && input.bytesInOutput > 0).length !== 1 ||
    inputs.some(input => input.path === 'web/report-viewer/app.js')) {
  throw new Error('The app must consume the canonical presentation module once, without the viewer host');
}
const script = emitted[0].contents;
if (/<\/script\b|sourceMappingURL|\beval\s*\(|\bnew\s+Function\b/.test(emitted[0].text)) {
  throw new Error('Unsafe inline bundle or source map');
}
const manifest = Buffer.from(JSON.stringify({
  version: 1,
  bundler: {name: 'esbuild', version},
  entryPoint: entry,
  inputs,
  configuration: await Promise.all(configuration.map(fileDigest)),
  styles: await Promise.all(styles.map(fileDigest)),
  output: digest(output, script),
}, null, 2) + '\n');
for (const [path, bytes] of [[output, script], [manifestPath, manifest]]) {
  const absolute = resolve(repository, path);
  if (check) {
    let committed;
    try { committed = await readFile(absolute); } catch { throw new Error(`Missing generated asset: ${path}. Run npm run build.`); }
    if (!committed.equals(bytes)) throw new Error(`Stale generated asset: ${path}. Run npm run build and commit both generated files.`);
  } else {
    await mkdir(dirname(absolute), {recursive: true});
    await writeFile(absolute, bytes);
  }
}
console.log(`${check ? 'Verified' : 'Generated'} report app: ${script.length} JavaScript bytes, ${inputs.length} authored modules, esbuild ${version}`);
