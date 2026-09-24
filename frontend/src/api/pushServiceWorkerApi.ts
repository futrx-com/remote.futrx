import { serviceWorkerTransport } from "../transport/serviceWorkerTransport.ts";
import { chatNotificationTag, PUSH_SERVICE_WORKER } from "../config/push.ts";

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

  /**
   * Removes a chat's notifications from the tray once the user is looking at
   * it. iOS never clears them on its own, so every turn would otherwise leave
   * one behind.
   */
  async closeChatNotifications(chatId: string): Promise<void> {
    try {
      const registration = await this.currentRegistration();
      if (!registration) return;
      const shown = await registration.getNotifications({ tag: chatNotificationTag(chatId) });
      for (const notification of shown) notification.close();
    } catch {
      // A leftover entry is harmless; tapping it just opens the chat.
    }
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
