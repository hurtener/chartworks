// Presentation-only MCP Apps host tokens, shared by both report adapters.
// No HTML, CSS rules, URLs, font downloads, or operation hints are accepted here.
const colors = {
  'background-primary': 'app-paper app-card bg',
  'background-secondary': 'app-rail soft app-input',
  'background-tertiary': 'app-raised',
  'background-info': 'app-soft',
  'text-primary': 'fg',
  'text-secondary': 'muted',
  'text-info': 'app-accent app-hover accent',
  'text-danger': 'danger',
  'border-primary': 'border',
};
const tokens = [
  ...Object.entries(colors).map(([key, names]) => ['--color-' + key, 'color', ...names.split(' ').map(name => '--' + name)]),
  ['--font-sans', 'font-family', '--app-font-sans'],
  ['--font-text-md-size', 'font-size', '--app-text-size'],
  ...['sm', 'md', 'lg'].map(size => ['--border-radius-' + size, 'border-radius', '--app-radius-' + size]),
];

export function applyHostTheme(root, context) {
  root.dataset.theme = context?.theme === 'dark' ? 'dark' : 'light';
  const variables = context?.styles?.variables;
  for (const [key, property, ...destinations] of tokens) {
    const candidate = variables?.[key];
    const value = typeof candidate === 'string' && candidate.length <= 512 &&
      !/[;{}<>\\]/.test(candidate) && !/\b(?:url|var|env)\s*\(/i.test(candidate) &&
      globalThis.CSS?.supports(property, candidate) ? candidate : '';
    // Empty values remove a previous host override and restore local defaults.
    for (const name of destinations) root.style.setProperty(name, value);
  }
}
