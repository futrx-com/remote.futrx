export interface AnsweredPair {
  question: string;
  answer: string;
}

class AnsweredText {
  /**
   * The question and answer pairs in an answered card's saved text: the
   * "Q: …\nA: …" blocks sent to the agent. Null for anything else, such as the
   * one-line preview older cards saved, which is then shown as it is.
   */
  parse(text: string): AnsweredPair[] | null {
    if (!text.startsWith("Q: ")) return null;
    const pairs: AnsweredPair[] = [];
    for (const block of text.split(/\n\n(?=Q: )/)) {
      const split = block.lastIndexOf("\nA: ");
      if (!block.startsWith("Q: ") || split < 0) return null;
      pairs.push({ question: block.slice(3, split), answer: block.slice(split + 4) });
    }
    return pairs;
  }
}

export const answeredText = new AnsweredText();
