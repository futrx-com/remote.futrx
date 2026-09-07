// The `remote.events` half of the extension API: the SPA announcing that
// something finished, so an image's `ui/` can react without polling and
// without patching anything global.
//
// Subscriptions are keyed by image id rather than held anonymously. That is
// what lets the extension host drop an uninstalled image's handlers: the
// module itself stays loaded in the page for the life of the tab, so a
// subscription nobody could reach would otherwise keep firing after the app
// it belongs to is gone.

import type {
  ExtensionEventHandler,
  ExtensionEventMap,
  ExtensionEventName,
} from "../../models/extension.ts";

interface Subscription {
  imageId: string;
  name: ExtensionEventName;
  // The handler is stored erased: on() has already checked the pairing of name
  // and handler at its call site, and emit() re-narrows on the way out.
  handler: (payload: never) => void;
}

class ExtensionEventService {
  private readonly subscriptions = new Set<Subscription>();

  /**
   * Registers one handler and returns its unsubscribe. Subscribing the same
   * handler twice registers it twice, which is what an image reloading its own
   * entry module would expect.
   */
  on<Name extends ExtensionEventName>(
    imageId: string,
    name: Name,
    handler: ExtensionEventHandler<Name>,
  ): () => void {
    const subscription: Subscription = {
      imageId,
      name,
      handler: handler as Subscription["handler"],
    };
    this.subscriptions.add(subscription);
    return () => {
      this.subscriptions.delete(subscription);
    };
  }

  /**
   * Delivers one event to every handler registered for it.
   *
   * Handlers are called synchronously and their results are discarded, so an
   * async handler runs on its own. A handler that throws is logged and the rest
   * still run: an extension must not be able to break the flow that emitted the
   * event, which here is a chat upload completing.
   */
  emit<Name extends ExtensionEventName>(
    name: Name,
    payload: ExtensionEventMap[Name],
  ): void {
    // Copied before iterating: a handler is free to unsubscribe itself, and
    // mutating the live set mid-emit would skip the handler after it.
    for (const subscription of [...this.subscriptions]) {
      if (subscription.name !== name) continue;
      try {
        (subscription.handler as (value: ExtensionEventMap[Name]) => void)(
          payload,
        );
      } catch (error) {
        console.error(
          `[extensions] ${subscription.imageId} failed handling ${name}`,
          error,
        );
      }
    }
  }

  /** Drops every subscription an image registered. Called when it is removed. */
  removeImage(imageId: string): void {
    for (const subscription of this.subscriptions) {
      if (subscription.imageId === imageId) {
        this.subscriptions.delete(subscription);
      }
    }
  }
}

export const extensionEventService = new ExtensionEventService();
