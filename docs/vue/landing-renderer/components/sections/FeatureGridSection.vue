<template>
  <section id="features" class="lp-section">
    <div class="lp-container">
      <div class="lp-section-heading lp-reveal">
        <p class="lp-eyebrow">Why this template works</p>
        <h2>{{ content.title }}</h2>
        <p>{{ content.description }}</p>
      </div>
      <div class="lp-card-grid">
        <article v-for="item in content.items || []" :key="item.title" class="lp-card lp-reveal">
          <div class="lp-card-icon">{{ iconLabel(item.icon) }}</div>
          <h3>{{ item.title }}</h3>
          <p>{{ item.description }}</p>
        </article>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  section: {
    type: Object,
    required: true,
  },
})

const content = computed(() => props.section.content || props.section.Content || {})

function iconLabel(icon) {
  const labels = {
    shield_check: 'SC',
    sparkles: 'AI',
    chart_bar: 'CX',
  }
  return labels[icon] || 'UX'
}
</script>
