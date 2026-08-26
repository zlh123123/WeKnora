import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./KnowledgeBase.vue', import.meta.url), 'utf8')

test('graph display mode dropdown lives in the active breadcrumb', () => {
  assert.match(source, /GraphModeSwitcherDropdown v-if="activeKbTab === 'graph'"/)
  assert.match(source, /graph-mode-breadcrumb/)
  assert.match(source, /knowledgeEditor\.wikiBrowser\.tabGraph/)
  assert.match(source, /name="chevron-down" class="graph-mode-chevron"/)
})

test('breadcrumb delegates privacy-aware mode changes to WikiBrowser', () => {
  assert.match(source, /wikiBrowserRef\.value\?\.requestGraphDisplayMode\(mode\)/)
  assert.match(source, /@graph-display-mode-change="onGraphDisplayModeChange"/)
})
