<template>
  <section class="lp-section lp-trust">
    <div class="lp-container">
      <p class="lp-trust-title">{{ content.title }}</p>
      <div class="lp-logo-row">
        <span v-for="logo in content.logos || []" :key="logo">{{ logo }}</span>
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
