export const SESSION_GROUP_VISIBLE_LIMIT = 5

// Expanding local rows and fetching another server page are separate actions.
export function sessionGroupPagination(loaded: number, visible: number, hasMorePages: boolean, total?: number) {
  // The server counts the whole filtered result while the list holds one page.
  // When a conversation is created between those reads, a group count can exceed
  // the rows that are loaded even though the project page reports the end of
  // history. Treat that as "still loadable": the group must keep the fetch
  // control so the counted row can be reconciled instead of silently vanishing
  // behind the header number.
  const hasMore = total === undefined ? hasMorePages : total > loaded
  const complete = visible >= loaded && !hasMore
  return {
    total: Math.max(loaded, total ?? loaded),
    complete,
    showControl: loaded > SESSION_GROUP_VISIBLE_LIMIT || hasMore,
    needsFetch: !complete && visible + SESSION_GROUP_VISIBLE_LIMIT > loaded && hasMore,
    nextVisible: complete ? SESSION_GROUP_VISIBLE_LIMIT : Math.min(visible + SESSION_GROUP_VISIBLE_LIMIT, hasMore ? (total ?? Infinity) : loaded),
  }
}
