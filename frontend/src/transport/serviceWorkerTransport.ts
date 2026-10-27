class ServiceWorkerTransport {
  get isSupported(): boolean {
    return typeof navigator !== "undefined" && "serviceWorker" in navigator;
  }

  register(scriptUrl: string, options: RegistrationOptions): Promise<ServiceWorkerRegistration> {
    return navigator.serviceWorker.register(scriptUrl, options);
  }

  registration(scope: string): Promise<ServiceWorkerRegistration | undefined> {
    return navigator.serviceWorker.getRegistration(scope);
  }

  ready(): Promise<ServiceWorkerRegistration> {
    return navigator.serviceWorker.ready;
  }

  listen(listener: (event: MessageEvent) => void): void {
    navigator.serviceWorker.addEventListener("message", listener);
  }
}

export const serviceWorkerTransport = new ServiceWorkerTransport();
