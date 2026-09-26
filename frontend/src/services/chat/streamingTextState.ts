type Presentation = "blocks" | "tokens";

export interface StreamingTextInput {
  text: string;
  streaming: boolean;
  presentation?: Presentation;
  hydrated?: boolean;
}

export interface StreamingTextState {
  text: string;
  requested: boolean;
  active: boolean;
  presentation: Presentation;
  hydrated: boolean;
}

// A text part keeps its first presentation mode and its hydration status.
// Starting the next turn must not re-stream an unchanged previous reply.
export function advanceStreamingTextState(
  previous: StreamingTextState | undefined,
  input: StreamingTextInput,
): StreamingTextState {
  if (!previous) {
    return {
      text: input.text,
      requested: input.streaming,
      active: input.streaming,
      presentation: input.presentation ?? "tokens",
      hydrated: input.hydrated ?? false,
    };
  }
  if (previous.hydrated || input.hydrated) return { ...previous, hydrated: true };

  let active = previous.active;
  if (!input.streaming || (!previous.requested && input.text === previous.text)) {
    active = false;
  } else if (input.text !== previous.text) {
    active = true;
  }
  return { ...previous, text: input.text, requested: input.streaming, active };
}
