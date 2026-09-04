// Why push notifications are unavailable, when they are. The UI explains each
// case differently: some are fixable by the user, some are not.
export type PushBlocker =
  | "unsupported" // no service worker or Push API in this browser
  | "install-required" // iOS: Web Push only works from the home screen
  | "denied" // the user (or the browser) refused permission
  | "server-disabled"; // the server has no VAPID key configured

export type PushStatus = "loading" | "blocked" | "off" | "on";

export interface PushConfig {
  enabled: boolean;
  publicKey: string;
  /** Whether this account already has at least one device registered. */
  subscribed: boolean;
}

/** The subset of PushSubscription.toJSON() the backend stores. */
export interface PushSubscriptionPayload {
  endpoint: string;
  keys: {
    p256dh: string;
    auth: string;
  };
}

export interface PushSubscriptionStatus {
  owned: boolean;
}

/** What one client reports it currently has on screen. */
export interface PushPresencePayload {
  /** The chat being watched, or "" when the user is not looking at one. */
  chatId: string;
  /** Identifies this tab, so one signing off cannot cancel another's claim. */
  clientId: string;
  /** Monotonically increases so the server can reject delayed older reports. */
  revision: number;
}

export interface PushPresenceStoreState {
  onScreenChatId: string | null;
  claimedChatId: string | null;
  revision: number;
}

export interface PushPresenceStoreActions {
  setWatchedChat: (chatId: string | null) => void;
}

type ChatOpener = (chatId: string | null) => void;

export interface PushNotificationStoreState {
  visibleChatId: string | null;
}

export interface PushNotificationStoreActions {
  connect: (openChat: ChatOpener) => void;
  setVisibleChat: (chatId: string | null) => void;
}
