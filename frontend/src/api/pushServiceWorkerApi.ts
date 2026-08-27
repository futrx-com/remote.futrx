import { serviceWorkerTransport } from "../transport/serviceWorkerTransport";
import { PUSH_SERVICE_WORKER } from "../config/push";

interface PushServiceWorkerCallbacks {
  visibleChatId: () => string | null;
  openChat: (chatId: string | null) => void;
}

class PushServiceWorkerApi {
  #callbacks: PushServiceWorkerCallbacks = {
    visibleChatId: () => null,
    openChat: () => {},
  };
  #isListening = false;

  get isSupported(): boolean {
    return serviceWorkerTransport.isSupported;
  }

  async register(): Promise<ServiceWorkerRegistration | null> {
    if (!this.isSupported) return null;
    this.#listen();
    try {
      return await serviceWorkerTransport.register(PUSH_SERVICE_WORKER.scriptUrl, {
        scope: PUSH_SERVICE_WORKER.scope,
      });
    } catch {
      return null;
    }
  }

  async currentRegistration(): Promise<ServiceWorkerRegistration | null> {
    if (!this.isSupported) return null;
    return (await serviceWorkerTransport.registration(PUSH_SERVICE_WORKER.scope)) ?? null;
  }

  ready(): Promise<ServiceWorkerRegistration> {
    return serviceWorkerTransport.ready();
  }

  connect(callbacks: PushServiceWorkerCallbacks): void {
    this.#callbacks = callbacks;
    this.#listen();
  }

  #listen(): void {
    if (this.#isListening || !this.isSupported) return;
    this.#isListening = true;

    serviceWorkerTransport.listen((event) => {
      const message = event.data;
      if (!message || typeof message !== "object") return;

      if (message.type === "which-chat") {
        event.ports[0]?.postMessage({ chatId: this.#callbacks.visibleChatId() });
        return;
      }

      if (message.type === "open-chat") {
        this.#callbacks.openChat(message.chatId ?? null);
      }
    });
  }
}

export const pushServiceWorkerApi = new PushServiceWorkerApi();
