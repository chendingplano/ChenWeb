import type { ElementInstance } from './types';

// Codes are shared with the save-error display. Optional empty values pass.
export function validateInput(value: string, d: Partial<ElementInstance>): string[] {
 const errors: string[] = [];
 if (d.binding_mode === 'read-only') return errors;
 if (value === '') return d.required ? ['required'] : [];
 const rules = d.validation ?? {};
 const length = Array.from(value).length;
 if (d.max_length !== undefined && length > d.max_length) errors.push('max_length');
 if (rules.min_length !== undefined && length < rules.min_length) errors.push('min_length');
 if (rules.pattern) {
  try { if (!new RegExp(`^(?:${rules.pattern})$(?![\\s\\S])`, 'u').test(value)) errors.push('pattern'); }
  catch { errors.push('pattern'); }
 }
 if (d.input_type === 'number') {
  const n = Number(value);
  if (!Number.isFinite(n) || value.trim() === '') errors.push('format');
  else {
   if (rules.min !== undefined && n < rules.min) errors.push('min');
   if (rules.max !== undefined && n > rules.max) errors.push('max');
   if (rules.step !== undefined) {
    const q = (n - (rules.min ?? 0)) / rules.step;
    if (Math.abs(q - Math.round(q)) > 1e-9) errors.push('step');
   }
  }
 } else if (d.input_type === 'email' && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) errors.push('format');
 else if (d.input_type === 'url') {
  try { const u = new URL(value); if (!['http:', 'https:'].includes(u.protocol)) errors.push('format'); }
  catch { errors.push('format'); }
 }
 return errors;
}
