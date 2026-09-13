import { WebSocketConnection } from "../transport/webSocketConnection";
import { webSocketUrl } from "../transport/webSocketUrl";
import type {
  TerminalConnection,
  TerminalConnectionCallbacks,
} from "../types/terminal";
import type { ApplicationPath } from "../types/transport";
import { WEB_SOCKET_ROUTES } from "../config/routes";
import {
  TERMINAL_MESSAGE_TYPES,
  TERMINAL_WEB_SOCKET_BINARY_TYPE,
} from "../config/terminal";

type TerminalClientMessage =
  | { type: typeof TERMINAL_MESSAGE_TYPES.input; data: string }
  | {
      type: typeof TERMINAL_MESSAGE_TYPES.resize;
      cols: number;
      rows: number;
    };

export const terminalApi = {
  connect(chatId: string, callbacks: TerminalConnectionCallbacks): TerminalConnection {
    return new WebSocketTerminalConnection(WEB_SOCKET_ROUTES.terminal(chatId), callbacks);
  },
  connectHost(callbacks: TerminalConnectionCallbacks): TerminalConnection {
    return new WebSocketTerminalConnection(WEB_SOCKET_ROUTES.hostTerminal, callbacks);
  },
};

class WebSocketTerminalConnection implements TerminalConnection {
  readonly #connection: WebSocketConnection;

  constructor(route: ApplicationPath, callbacks: TerminalConnectionCallbacks) {
    this.#connection = new WebSocketConnection({
      url: webSocketUrl(route),
      binaryType: TERMINAL_WEB_SOCKET_BINARY_TYPE,
      onOpen: callbacks.onOpen,
      onMessage(data) {
        callbacks.onOutput(
          data instanceof ArrayBuffer ? new Uint8Array(data) : String(data)
        );
      },
      onError: callbacks.onError,
      onClose: callbacks.onClose,
    });
  }

  get isOpen(): boolean {
    return this.#connection.isOpen;
  }

  sendInput(data: string): void {
    this.#send({ type: TERMINAL_MESSAGE_TYPES.input, data });
  }

  resize(cols: number, rows: number): void {
    this.#send({ type: TERMINAL_MESSAGE_TYPES.resize, cols, rows });
  }

  close(): void {
    this.#connection.close();
  }

  #send(message: TerminalClientMessage): void {
    if (!this.isOpen) return;
    this.#connection.send(JSON.stringify(message));
  }
}
