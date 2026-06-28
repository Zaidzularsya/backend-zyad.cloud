<template>
  <main class="lp-shell" :style="themeVars">
    <SectionRenderer
      v-for="section in resolvedSections"
      :key="section.id || section.ID || section.key || section.Key"
      :section="section"
    />
  </main>
</template>

<script setup>
import { computed } from 'vue'
import SectionRenderer from './SectionRenderer.vue'
import '../landing-renderer.css'

const props = defineProps({
  resolved: {
    type: Object,
    required: true,
  },
})

const snapshot = computed(() => props.resolved?.snapshot || props.resolved?.Snapshot || {})
const sections = computed(() => props.resolved?.sections || props.resolved?.Sections || [])
const branding = computed(() => props.resolved?.branding || props.resolved?.Branding || {})

const resolvedSections = computed(() => {
  const snapshotSections = snapshot.value?.sections || snapshot.value?.Sections
  return Array.isArray(snapshotSections) && snapshotSections.length > 0 ? snapshotSections : sections.value
})

const colors = computed(() => {
  const brandingColors = branding.value?.colors || branding.value?.Colors
  const firstSection = resolvedSections.value[0] || {}
  const style = firstSection.style || firstSection.Style || {}
  return brandingColors || style.colors || {}
})

const themeVars = computed(() => ({
  '--lp-primary': colors.value.primary || '#2563eb',
  '--lp-secondary': colors.value.secondary || '#0f172a',
  '--lp-accent': colors.value.accent || colors.value.primary || '#2563eb',
  '--lp-bg': colors.value.background || '#ffffff',
  '--lp-surface': colors.value.surface || '#f8fafc',
  '--lp-text': colors.value.text || '#0f172a',
  '--lp-muted': colors.value.muted || '#64748b',
}))
</script>
