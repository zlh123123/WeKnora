import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./WikiBrowser.vue', import.meta.url), 'utf8')

test('graph mode controller and legends keep ordinary and personal views separate', () => {
  assert.match(source, /defineExpose\(\{ requestGraphDisplayMode \}\)/)
  assert.doesNotMatch(source, /class="graph-mode-switcher"/)
  assert.match(source, /graphDisplayMode === 'knowledge'/)
  assert.match(source, /learningLegendEntries/)
  assert.match(source, /summary: '#0052d9', entity: '#2ba471', concept: '#e37318'/)
})

test('personal mode uses the batched overlay and never sends a subject identifier', () => {
  assert.match(source, /getLearningOverlay\(props\.knowledgeBaseId\)/)
  assert.doesNotMatch(source, /getLearningOverlay\([^)]*subject/i)
  assert.match(source, /node\.type === 'concept' \? learningStatusColor\(node\.learningStatus\)/)
})

test('concept drawer preserves page content below the learning status card', () => {
  const card = source.indexOf('wiki-learning-status-card')
  const body = source.indexOf('class="wiki-reader-body"', card)
  assert.ok(card >= 0)
  assert.ok(body > card)
  assert.match(source, /EMPTY_LEARNING_STATE/)
})
