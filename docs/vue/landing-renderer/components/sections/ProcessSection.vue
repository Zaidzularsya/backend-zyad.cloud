<template>
  <section class="lp-section">
    <div class="lp-container">
      <div class="lp-section-heading lp-reveal">
        <p class="lp-eyebrow">Conversion flow</p>
        <h2>{{ content.title }}</h2>
      </div>
      <div class="lp-process">
        <article v-for="step in content.steps || []" :key="step.number" class="lp-process-step lp-reveal">
          <span>{{ step.number }}</span>
          <h3>{{ step.title }}</h3>
          <p>{{ step.description }}</p>
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
</script>
