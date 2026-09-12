// Normalizing text so two spellings of one word compare equal.
//
// Folding is deliberately length-preserving: every transform maps one source
// character to exactly one output character, so an offset into the folded
// string is a valid offset into the original. That is what lets a caller fold
// once at index time and still emit highlight spans against the raw text.
//
// It is separated from matching because the two change for different reasons:
// this file changes when a script or a spelling variant needs handling, the
// matcher when ranking is tuned. Find-in-chat also folds without ever scoring.
//
// Leaf service: it knows about the language, never about chats or filters.

import { FOLD_CACHE_LIMIT } from "../../config/search.ts";

/**
 * One-to-one character equivalences that NFD cannot express, so a word matches
 * however the writer happened to spell it. Every entry maps a single code point
 * to a single code point, which is what keeps folding length-preserving.
 *
 * The Arabic set is the usual orthographic drift: alef maksura written for yeh,
 * teh marbuta for heh, and the Persian/Urdu keheh and yeh for their Arabic
 * counterparts. Hamza carriers (أ إ آ ؤ ئ) already fold through NFD.
 */
function buildCharEquivalents(): ReadonlyMap<string, string> {
  const equivalents = new Map([
    ["ى", "ي"], // alef maksura -> yeh
    ["ة", "ه"], // teh marbuta -> heh
    ["ی", "ي"], // farsi yeh -> yeh
    ["ک", "ك"], // keheh -> kaf
    ["ڪ", "ك"], // swash kaf -> kaf
    ["ٰ", "ا"], // superscript alef -> alef
  ]);
  // Arabic-Indic and extended Arabic-Indic digits, so "٥" and "۵" find "5".
  for (const zero of [0x0660, 0x06f0]) {
    for (let digit = 0; digit <= 9; digit += 1) {
      equivalents.set(String.fromCodePoint(zero + digit), String(digit));
    }
  }
  return equivalents;
}

const CHAR_EQUIVALENTS = buildCharEquivalents();

class TextFoldService {
  readonly #cache = new Map<string, string>();

  /**
   * Lowercase, strip diacritics and settle script-specific spelling variants,
   * leaving the string exactly as long as it arrived so span offsets stay
   * aligned with the original text. Memoized, since a field is folded once at
   * index time and then compared on every keystroke.
   */
  fold(value: string): string {
    if (!value) return "";
    const cached = this.#cache.get(value);
    if (cached !== undefined) return cached;

    let out = "";
    for (const char of value) out += this.#foldChar(char);
    // Bounded so a long session cannot grow the cache without limit.
    if (this.#cache.size < FOLD_CACHE_LIMIT) this.#cache.set(value, out);
    return out;
  }

  /**
   * One character in, one character of the same length out -- the invariant the
   * rest of the file rests on, in the one place it can be checked.
   *
   * NFD splits an accented char into base + combining marks, so taking the base
   * strips the accent. Any transform that would change the length (lowercasing
   * "İ" to "i̇", decomposing an astral char) is refused and the char stands.
   */
  #foldChar(char: string): string {
    const base = char.toLowerCase().normalize("NFD")[0] ?? char;
    const stripped = base.length === char.length ? base : char;
    return CHAR_EQUIVALENTS.get(stripped) ?? stripped;
  }
}

export const textFoldService = new TextFoldService();
