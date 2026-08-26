import type { LearningOverlayItem, LearningStatus } from '@/api/learning'

export const LEARNING_STATUS_COLORS: Record<LearningStatus, string> = {
  unseen: '#8b95a5',
  exposed: '#3b82f6',
  uncertain: '#f59e0b',
  verified_strong: '#10b981',
  verified_weak: '#ef4444',
}

export const EMPTY_LEARNING_STATE: Readonly<LearningOverlayItem> = {
  concept_key: '',
  wiki_page_id: '',
  slug: '',
  status: 'unseen',
  exposure_count: 0,
  exposure_weight: 0,
  verified_mastery: 0,
  mastery_confidence: 0,
  quiz_attempt_count: 0,
}

export function normalizeLearningStatus(status?: string): LearningStatus {
  if (status === 'exposed' || status === 'uncertain' || status === 'verified_strong' || status === 'verified_weak') {
    return status
  }
  return 'unseen'
}

export function buildLearningOverlayIndexes(items: LearningOverlayItem[]) {
  const bySlug = new Map<string, LearningOverlayItem>()
  const byPageID = new Map<string, LearningOverlayItem>()
  for (const item of items) {
    const normalized = { ...item, status: normalizeLearningStatus(item.status) }
    if (normalized.slug) bySlug.set(normalized.slug, normalized)
    if (normalized.wiki_page_id) byPageID.set(normalized.wiki_page_id, normalized)
  }
  return { bySlug, byPageID }
}
