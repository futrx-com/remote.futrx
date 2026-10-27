type StandaloneNavigator = Navigator & { standalone?: boolean };

class WebPushTransport {
  get isPushManagerSupported(): boolean {
    return "PushManager" in window;
  }

  get isNotificationSupported(): boolean {
    return "Notification" in window;
  }

  get notificationPermission(): NotificationPermission {
    return Notification.permission;
  }

  requestPermission(): Promise<NotificationPermission> {
    return Notification.requestPermission();
  }

  currentSubscription(
    registration: ServiceWorkerRegistration
  ): Promise<PushSubscription | null> {
    return registration.pushManager.getSubscription();
  }

  subscribe(
    registration: ServiceWorkerRegistration,
    applicationServerKey: string
  ): Promise<PushSubscription> {
    return registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: this.#decodeBase64Url(applicationServerKey),
    });
  }

  unsubscribe(subscription: PushSubscription): Promise<boolean> {
    return subscription.unsubscribe();
  }

  /**
   * Whether this subscription was provably signed with a *different* key than
   * the server signs with today, which makes it undeliverable.
   *
   * A browser that does not expose the applied key answers false: an
   * unprovable mismatch must never cost a working subscription, because
   * unsubscribing is exactly what drops the notification permission on Safari
   * and puts the "Allow notifications?" prompt back in front of the user.
   */
  isSignedWithRetiredKey(
    subscription: PushSubscription,
    applicationServerKey: string
  ): boolean {
    const appliedKey = subscription.options?.applicationServerKey;
    if (!appliedKey) return false;
    return (
      this.#encodeBase64Url(new Uint8Array(appliedKey)) !==
      applicationServerKey.replace(/=+$/, "")
    );
  }

  isIOS(): boolean {
    return (
      /iPad|iPhone|iPod/.test(navigator.userAgent) ||
      // iPadOS reports itself as a Mac, but is the only "Mac" with touch.
      (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1)
    );
  }

  isStandalone(): boolean {
    return (
      window.matchMedia("(display-mode: standalone)").matches ||
      (window.navigator as StandaloneNavigator).standalone === true
    );
  }

  /** VAPID keys travel as base64url; PushManager wants the raw bytes. */
  #decodeBase64Url(value: string): Uint8Array<ArrayBuffer> {
    const padded = value.replace(/-/g, "+").replace(/_/g, "/");
    const binary = atob(
      padded.padEnd(padded.length + ((4 - (padded.length % 4)) % 4), "=")
    );
    const bytes = new Uint8Array(new ArrayBuffer(binary.length));
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes;
  }

  #encodeBase64Url(bytes: Uint8Array): string {
    let binary = "";
    for (const byte of bytes) binary += String.fromCharCode(byte);
    return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }
}

export const webPushTransport = new WebPushTransport();
