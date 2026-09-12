/**
 * Text helpers with no domain of their own, kept here so every layer can reach
 * them: `config/` is a leaf that `services/`, `state/` and `ui/` all import,
 * and one of the five copies of this rule lived in `config/chat.ts` itself, so
 * a service could not have owned it without inverting the direction.
 */

/** Upper-case the first character and leave the rest alone. */
export function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
