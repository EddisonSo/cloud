// Centralized cache management for API data
// Each hook registers its cache clear function here

const clearFunctions: (() => void)[] = [];

export function registerCacheClear(fn: () => void): void {
  clearFunctions.push(fn);
}

export function clearAllCaches(): void {
  clearFunctions.forEach((fn) => fn());
}
