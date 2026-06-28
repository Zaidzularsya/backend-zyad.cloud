<template>
  <section class="lp-section">
    <div class="lp-container lp-card">
      <p class="lp-eyebrow">{{ section.type || section.Type || 'section' }}</p>
      <h2>{{ content.title || section.name || section.Name }}</h2>
      <p>{{ content.description }}</p>
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
</script>
