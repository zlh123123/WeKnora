import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./WikiBrowser.vue', import.meta.url), 'utf8')

test('personal knowledge map exposes a resumable scan without a new page', () => {
  assert.match(source, /learningScanStart/)
  assert.match(source, /openLearningScan/)
  assert.match(source, /getActiveLearningScan\(props\.knowledgeBaseId\)/)
  assert.match(source, /startOrResumeLearningScan\(props\.knowledgeBaseId\)/)
  assert.match(source, /submitLearningQuizAttempt/)
  assert.match(source, /completeLearningScan/)
  assert.match(source, /fetchLearningOverlay\(\)/)
  assert.doesNotMatch(source, /subject_id.*learning/i)
})

test('an empty active-scan response unwraps null instead of becoming a fake scan', () => {
  assert.match(source, /typeof response === 'object' && 'data' in response/)
  assert.doesNotMatch(source, /\(response as any\)\?\.data \|\| response/)
})

test('scan answer rows use the vertical radio layout with one shared left edge', () => {
  assert.match(source, /<t-radio-group[^>]*direction="vertical"[^>]*class="learning-scan-options"/)
  assert.match(source, /\.learning-scan-options\s*\{[^}]*align-items:\s*stretch;[^}]*width:\s*100%;/s)
  assert.match(source, /\.learning-scan-options :deep\(\.t-radio\)\s*\{[^}]*align-items:\s*center;[^}]*width:\s*100%;[^}]*min-height:\s*56px;[^}]*margin-right:\s*0;/s)
  assert.match(source, /class="learning-scan-option-content"/)
  assert.match(source, /\.learning-scan-option-content\s*\{[^}]*align-items:\s*center;[^}]*gap:\s*14px;/s)
  assert.match(source, /width="min\(620px, calc\(100vw - 32px\)\)"/)
  assert.match(source, /\.wiki-learning-scan-dialog \.t-dialog\)\s*\{[^}]*max-width:\s*calc\(100vw - 32px\);[^}]*overflow:\s*hidden;/s)
})
