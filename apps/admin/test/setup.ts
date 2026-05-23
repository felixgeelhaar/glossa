// Test setup: shim localStorage. happy-dom doesn't expose it
// reliably across versions, and we only use the four-method
// surface — a Map-backed stub is enough.

class MemoryStorage {
  private store = new Map<string, string>();
  public get length(): number {
    return this.store.size;
  }
  public clear(): void {
    this.store.clear();
  }
  public getItem(key: string): string | null {
    return this.store.get(key) ?? null;
  }
  public setItem(key: string, value: string): void {
    this.store.set(key, value);
  }
  public removeItem(key: string): void {
    this.store.delete(key);
  }
  public key(i: number): string | null {
    return Array.from(this.store.keys())[i] ?? null;
  }
}

if (typeof globalThis.localStorage === "undefined") {
  Object.defineProperty(globalThis, "localStorage", { value: new MemoryStorage() });
}
if (typeof globalThis.window !== "undefined" && !globalThis.window.localStorage) {
  Object.defineProperty(globalThis.window, "localStorage", { value: globalThis.localStorage });
}
