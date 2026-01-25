// Centralized cache management for API data
// Each hook registers its cache clear function here

const clearFunctions = [];

export function registerCacheClear(fn) {
  clearFunctions.push(fn);
}

export function clearAllCaches() {
  clearFunctions.forEach((fn) => fn());
}
