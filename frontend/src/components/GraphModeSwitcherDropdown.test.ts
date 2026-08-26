import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./GraphModeSwitcherDropdown.vue', import.meta.url), 'utf8')

test('graph mode switcher mirrors the knowledge-base dropdown selection grammar', () => {
  assert.match(source, /v-model:visible="visible"/)
  assert.match(source, /:class="\{ active: item\.value === current \}"/)
  assert.match(source, /v-if="item\.value === current"/)
  assert.match(source, /name="check"/)
  assert.match(source, /graph-mode-switcher-row-icon/)
})

test('graph mode switcher closes before emitting a changed selection', () => {
  assert.match(source, /visible\.value = false/)
  assert.match(source, /if \(mode === props\.current\) return/)
  assert.match(source, /emit\('select', mode\)/)
})
