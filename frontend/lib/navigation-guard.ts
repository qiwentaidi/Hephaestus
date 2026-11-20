type NavigationGuard = () => boolean | Promise<boolean>

const guards = new Set<NavigationGuard>()

/**
 * Register a guard that can veto or asynchronously confirm in-app navigation.
 * The returned function unregisters it.
 */
export function registerNavigationGuard(guard: NavigationGuard): () => void {
  guards.add(guard)
  return () => {
    guards.delete(guard)
  }
}

/**
 * Resolve whether the current navigation may proceed. Guards run in
 * registration order; the first one that returns false stops the navigation.
 */
export async function confirmNavigation(): Promise<boolean> {
  for (const guard of Array.from(guards)) {
    try {
      if (!(await guard())) return false
    } catch {
      return false
    }
  }
  return true
}
