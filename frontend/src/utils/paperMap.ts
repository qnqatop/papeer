import { ListPapers } from '../../wailsjs/go/app/App'
import { db } from '../../wailsjs/go/models'

/**
 * Fetches every approved/downloaded paper for a profile as an id -> Paper
 * map. Mirrors the backend's `approvedAndDownloadedFull` eligibility set
 * (internal/app/app_topics.go) — Topics paper_ids, Key Papers rows and
 * Coverage Gaps' mentioned_by all reference ids from this same set, so a
 * single shared fetch here keeps the three tabs consistent and avoids
 * duplicating the merge logic per component.
 */
export async function fetchApprovedDownloadedMap(profileId: number): Promise<Map<number, db.Paper>> {
  const [approved, downloaded] = await Promise.all([
    ListPapers(new db.PaperFilter({ profile_id: profileId, status: 'approved', limit: 10000, offset: 0 })),
    ListPapers(new db.PaperFilter({ profile_id: profileId, status: 'downloaded', limit: 10000, offset: 0 })),
  ])
  const map = new Map<number, db.Paper>()
  for (const p of [...(approved.papers || []), ...(downloaded.papers || [])]) {
    map.set(p.id, p)
  }
  return map
}
