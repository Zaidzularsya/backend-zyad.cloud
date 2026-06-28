<template>
  <section class="lp-section lp-stat-band">
    <div class="lp-container lp-stat-grid">
      <article v-for="item in content.items || []" :key="item.label" class="lp-stat lp-reveal">
        <strong>{{ item.value }}</strong>
        <span>{{ item.label }}</span>
        <p>{{ item.description }}</p>
      </article>
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
