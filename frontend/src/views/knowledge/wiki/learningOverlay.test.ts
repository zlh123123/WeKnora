import assert from 'node:assert/strict'
import test from 'node:test'

import type { LearningOverlayItem } from '@/api/learning'
import {
  EMPTY_LEARNING_STATE,
  LEARNING_STATUS_COLORS,
  buildLearningOverlayIndexes,
  normalizeLearningStatus,
} from './learningOverlay.ts'

const item = (overrides: Partial<LearningOverlayItem> = {}): LearningOverlayItem => ({
  concept_key: 'concept:rag',
  wiki_page_id: 'page-rag',
  slug: 'rag',
  status: 'verified_strong',
  exposure_count: 2,
  exposure_weight: 1,
  verified_mastery: 0.9,
  mastery_confidence: 0.8,
  quiz_attempt_count: 2,
  ...overrides,
})

test('unknown or missing learning state is rendered as unseen', () => {
  assert.equal(normalizeLearningStatus(), 'unseen')
  assert.equal(normalizeLearningStatus('legacy_value'), 'unseen')
  assert.equal(EMPTY_LEARNING_STATE.status, 'unseen')
})

test('overlay is indexed once for graph slugs and drawer page IDs', () => {
  const source = item()
  const { bySlug, byPageID } = buildLearningOverlayIndexes([source])

  assert.equal(bySlug.get('rag')?.status, 'verified_strong')
  assert.equal(byPageID.get('page-rag')?.concept_key, 'concept:rag')
})

test('personal graph exposes exactly the five approved mastery colors', () => {
  assert.deepEqual(Object.keys(LEARNING_STATUS_COLORS).sort(), [
    'exposed',
    'uncertain',
    'unseen',
    'verified_strong',
    'verified_weak',
  ])
})
