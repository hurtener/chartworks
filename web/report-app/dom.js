// Shared text-only controls. Values and labels never pass through an HTML parser.
// Feature modules retain their own state, availability checks and event handlers.
export function node(tag, text, className) {
  const element = document.createElement(tag);
  if (text !== undefined) element.textContent = String(text);
  if (className) element.className = className;
  return element;
}

export function button(label, onClick, disabled = false) {
  const element = node('button', label);
  element.type = 'button';
  element.disabled = disabled;
  element.addEventListener('click', onClick);
  return element;
}

export function selectField(label, items, value, onChange, disabled = false) {
  const field = node('label', label), select = node('select');
  select.disabled = disabled;
  select.setAttribute('aria-label', label);
  for (const item of items) {
    const option = node('option', item.label);
    option.value = item.value;
    option.selected = item.value === value;
    option.disabled = !!item.disabled;
    select.append(option);
  }
  select.addEventListener('change', () => onChange(select.value));
  field.append(select);
  return field;
}

export function textField(label, value, onChange, {
  type = 'text', maxLength = 256, disabled = false, min, max,
} = {}) {
  const field = node('label', label), input = node(type === 'textarea' ? 'textarea' : 'input');
  if (type !== 'textarea') input.type = type;
  input.value = value ?? '';
  input.defaultValue = input.value;
  input.maxLength = maxLength;
  input.disabled = disabled;
  if (min !== undefined) input.min = min;
  if (max !== undefined) input.max = max;
  input.setAttribute('aria-label', label);
  input.addEventListener('change', () => onChange(input.value));
  field.append(input);
  return field;
}
