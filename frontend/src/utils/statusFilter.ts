// willLeaveStatusFilter predicts whether a paper whose triage status changes
// to newStatus drops out of the list filtered by statusFilter. An empty or
// missing filter means the "All" tab, where every paper stays visible.
export function willLeaveStatusFilter(
  statusFilter: string | null | undefined,
  newStatus: string,
): boolean {
  return !!statusFilter && statusFilter !== newStatus
}
