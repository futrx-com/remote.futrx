import { useRef } from "preact/hooks";
import {
  advanceStreamingTextState,
  type StreamingTextInput,
  type StreamingTextState,
} from "../../../services/chat/streamingTextState";

export function useStreamingTextState(input: StreamingTextInput): StreamingTextState {
  const state = useRef<StreamingTextState>();
  state.current = advanceStreamingTextState(state.current, input);
  return state.current;
}
