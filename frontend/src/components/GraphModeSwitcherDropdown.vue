<template>
  <t-popup
    v-model:visible="visible"
    trigger="click"
    placement="bottom-left"
    :overlay-style="{ padding: 0 }"
    :overlay-inner-style="{ padding: 0 }"
  >
    <template #content>
      <div class="graph-mode-switcher-card">
        <div class="graph-mode-switcher-list">
          <button
            v-for="item in modes"
            :key="item.value"
            type="button"
            class="graph-mode-switcher-row"
            :class="{ active: item.value === current }"
            @click="handleSelect(item.value)"
          >
            <t-icon :name="item.icon" class="graph-mode-switcher-row-icon" size="16px" />
            <span class="graph-mode-switcher-row-name">{{ item.label }}</span>
            <t-icon
              v-if="item.value === current"
              name="check"
              class="graph-mode-switcher-row-check"
              size="14px"
            />
          </button>
        </div>
      </div>
    </template>
    <slot />
  </t-popup>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

export type GraphDisplayMode = 'knowledge' | 'learning'

const props = defineProps<{
  current: GraphDisplayMode
}>()

const emit = defineEmits<{
  (e: 'select', mode: GraphDisplayMode): void
}>()

const { t } = useI18n()
const visible = ref(false)
const modes = computed(() => [
  {
    value: 'knowledge' as const,
    label: t('knowledgeEditor.wikiBrowser.graphModeKnowledge'),
    icon: 'chart-bubble',
  },
  {
    value: 'learning' as const,
    label: t('knowledgeEditor.wikiBrowser.graphModeLearning'),
    icon: 'user',
  },
])

const handleSelect = (mode: GraphDisplayMode): void => {
  visible.value = false
  if (mode === props.current) return
  emit('select', mode)
}
</script>

<style scoped lang="less">
/* Keep this visual grammar intentionally identical to KBSwitcherDropdown. */
.graph-mode-switcher-card {
  min-width: 220px;
  max-width: 320px;
  max-height: min(60vh, 420px);
  display: flex;
  flex-direction: column;
  padding: 6px;
  overflow: hidden;
}

.graph-mode-switcher-list {
  flex: 1 1 auto;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.graph-mode-switcher-row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--td-text-color-primary);
  font-size: 13px;
  line-height: 1.4;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
  text-align: left;

  &:hover {
    background: var(--td-bg-color-secondarycontainer);
  }

  &.active {
    background: var(--td-brand-color-light, rgba(0, 82, 217, 0.08));
    color: var(--td-brand-color);
    font-weight: 500;
  }
}

.graph-mode-switcher-row-icon {
  flex: 0 0 auto;
  color: var(--td-text-color-placeholder);

  .graph-mode-switcher-row.active & {
    color: var(--td-brand-color);
  }
}

.graph-mode-switcher-row-name {
  flex: 1 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.graph-mode-switcher-row-check {
  flex: 0 0 auto;
  color: var(--td-brand-color);
}
</style>
