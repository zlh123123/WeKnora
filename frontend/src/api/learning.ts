import { get, post, put, getDown } from '../utils/request'

export type LearningStatus = 'unseen' | 'exposed' | 'uncertain' | 'verified_strong' | 'verified_weak'

export interface LearningOverlayItem {
  concept_key: string
  wiki_page_id: string
  slug: string
  status: LearningStatus
  exposure_count: number
  exposure_weight: number
  verified_mastery: number
  mastery_confidence: number
  quiz_attempt_count: number
  last_exposed_at?: string
  last_assessed_at?: string
}

export interface LearningOverlay {
  tracking_enabled: boolean
  items: LearningOverlayItem[]
}

export interface LearningScanItem {
  id: string
  concept_key: string
  concept_title: string
  question: string
  options: string[]
}

export interface LearningScan {
  id: string
  status: 'pending' | 'active' | 'completed'
  current_index: number
  total_items: number
  items: LearningScanItem[]
  summary: { verified_strong: number; verified_weak: number; uncertain: number }
}

export interface LearningEvidenceView {
  id: string
  event_type: string
  confidence: number
  evidence_value: number
  occurred_at: string
  session_id?: string
  message_id?: string
  chunk_id?: string
  quiz_item_id?: string
  quiz_attempt_id?: string
}

export interface LearningRecommendation {
  concept_key: string
  title: string
  slug: string
  reason: string
}

export interface LearningConceptInsights {
  concept_key: string
  status: LearningStatus
  is_knowledge_gap: boolean
  gap_reason?: string
  evidence: LearningEvidenceView[]
  recommendation?: LearningRecommendation
}

export function getLearningOverlay(kbId: string) {
  return get(`/api/v1/knowledgebase/${kbId}/wiki/learning-overlay`)
}

export function getLearningConceptInsights(kbId: string, conceptKey: string) {
  return get(`/api/v1/knowledgebase/${kbId}/learning/concepts/${encodeURIComponent(conceptKey)}/insights`)
}

export function setLearningTracking(kbId: string, enabled: boolean) {
  return put(`/api/v1/knowledgebase/${kbId}/learning/tracking`, { enabled })
}

export function exportLearningProfile(kbId: string): Promise<Blob> {
  return getDown(`/api/v1/knowledgebase/${kbId}/learning/export`)
}

export function startOrResumeLearningScan(kbId: string) {
  // A cold scan may need to generate grounded quiz banks for three concepts.
  // Keep the longer timeout local to this model-backed request.
  return post(`/api/v1/knowledgebase/${kbId}/learning/scans`, {}, { timeout: 3 * 60 * 1000 })
}

export function getActiveLearningScan(kbId: string) {
  return get(`/api/v1/knowledgebase/${kbId}/learning/scans/active`)
}

export function submitLearningQuizAttempt(kbId: string, itemId: string, payload: {
  scan_id: string
  selected_option: number
  idempotency_key: string
}) {
  return post(`/api/v1/knowledgebase/${kbId}/learning/quiz/${itemId}/attempt`, payload)
}

export function completeLearningScan(kbId: string, scanId: string) {
  return post(`/api/v1/knowledgebase/${kbId}/learning/scans/${scanId}/complete`)
}
