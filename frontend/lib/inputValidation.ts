const controlCharacter = /[\u0000-\u001F\u007F]/u;
// Plain-text fields do not support markup. This deliberately looks for a tag
// shape rather than banning every less-than character in every input type.
const markup = /<\s*[!/?A-Za-z][^>]*>?/u;

export function normalizePlainText(value: string): string {
  return value.trim();
}

export function plainTextError(
  value: string,
  label: string,
  options: { min?: number; max: number; required?: boolean },
): string | null {
  const normalized = normalizePlainText(value);
  if (options.required && !normalized) return `${label} ist erforderlich.`;
  if (!normalized) return null;
  if (normalized.length < (options.min ?? 1)) return `${label} ist zu kurz.`;
  if (normalized.length > options.max) return `${label} ist zu lang.`;
  if (controlCharacter.test(normalized) || markup.test(normalized)) {
    return `${label} darf keine Steuerzeichen oder HTML-Markup enthalten.`;
  }
  return null;
}

export function emailError(value: string): string | null {
  const normalized = value.trim();
  if (!normalized || normalized.length > 254) return "Bitte geben Sie eine gültige E-Mail-Adresse ein.";
  // This is early UX validation only; Identity owns the authoritative email policy.
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/u.test(normalized)) return "Bitte geben Sie eine gültige E-Mail-Adresse ein.";
  return null;
}

export function passwordError(value: string, requireStrongLength: boolean): string | null {
	if (!value) return "Passwort ist erforderlich.";
	if (new TextEncoder().encode(value).length > 72) return "Passwörter dürfen höchstens 72 Bytes lang sein.";
  if (requireStrongLength && value.length < 15) return "Das Passwort muss mindestens 15 Zeichen lang sein.";
  return null;
}
