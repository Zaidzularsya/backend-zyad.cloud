<template>
  <section id="contact" class="lp-section lp-final-cta">
    <div class="lp-container lp-cta-panel lp-reveal">
      <h2>{{ content.title }}</h2>
      <p>{{ content.description }}</p>
      <div class="lp-actions">
        <a class="lp-button lp-button-primary" href="#lead-form">{{ content.primaryButtonText }}</a>
        <a class="lp-button lp-button-ghost" href="#features">{{ content.secondaryButtonText }}</a>
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
